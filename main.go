package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	pushoverURL     = "https://api.pushover.net/1/messages.json"
	serverPort      = ":8080"
	readTimeout     = 10 * time.Second
	writeTimeout    = 10 * time.Second
	shutdownTimeout = 30 * time.Second
	maxBodySize     = 1 << 20 // 1MB

	// Gyakran használt stringek konstansként
	defaultSeverity = "INFO"
	defaultValue    = "Unknown"
	noMessage       = "No Message"
	appTitle        = "FluxCD"
	
	// HTTP related constants
	contentTypeJSON = "application/json"
	contentTypeForm = "application/x-www-form-urlencoded"
	bearerPrefix    = "Bearer "
)

// Előre definiált JSON válaszok
var (
	responseOK           = []byte(`{"status": "ok"}`)
	responseUnauthorized = []byte(`{"error": "Unauthorized"}`)
	responseInvalidJSON  = []byte(`{"error": "Invalid JSON"}`)
	responseRootError    = []byte("Requests need to be made to /webhook")
	responseHealthy      = []byte("healthy")
)

// String builder pool
var builderPool = sync.Pool{
	New: func() interface{} {
		b := &strings.Builder{}
		b.Grow(256)
		return b
	},
}

// Config holds application configuration
type Config struct {
	PushoverUserKey  string
	PushoverAPIToken string
	BearerToken      string // Előre kiszámított Bearer token
}

// FluxAlert
type FluxAlert struct {
	InvolvedObject struct {
		Kind            string `json:"kind"`
		Namespace       string `json:"namespace"`
		Name            string `json:"name"`
		UID             string `json:"uid"`
		APIVersion      string `json:"apiVersion"`
		ResourceVersion string `json:"resourceVersion"`
	} `json:"involvedObject"`
	Severity  string `json:"severity"`
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Reason    string `json:"reason"`
	Metadata  struct {
		CommitStatus string `json:"commit_status"`
		Revision     string `json:"revision"`
		Summary      string `json:"summary"`
	} `json:"metadata"`
	ReportingController string `json:"reportingController"`
	ReportingInstance   string `json:"reportingInstance"`
}

// Server represents the HTTP server
type Server struct {
	config *Config
	client *http.Client
}

// NewServer creates a new server instance with optimized HTTP client
func NewServer(config *Config) *Server {
	// HTTP client beállítások
	transport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true, // Pushover nem használ compression-t
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	
	return &Server{
		config: config,
		client: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
	}
}

// handleRoot handles requests to the root path
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusBadRequest)
	w.Write(responseRootError)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write(responseHealthy)
}

// handleWebhook handles incoming webhook requests from Flux
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// Előre kiszámított Bearer token használata
	if r.Header.Get("Authorization") != s.config.BearerToken {
		log.Printf("Unauthorized request from %s", r.RemoteAddr)
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(responseUnauthorized)
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	// Parse JSON payload
	var alert FluxAlert
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&alert); err != nil {
		log.Printf("Failed to parse JSON: %v", err)
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(responseInvalidJSON)
		return
	}
	defer r.Body.Close()

	// Pool-ból vett string builder
	msgBuilder := builderPool.Get().(*strings.Builder)
	defer func() {
		msgBuilder.Reset()
		builderPool.Put(msgBuilder)
	}()

	// Build message with optimized string operations
	severity := defaultSeverity
	if alert.Severity != "" {
		severity = strings.ToUpper(alert.Severity)
	}

	reason := defaultValue
	if alert.Reason != "" {
		reason = alert.Reason
	}

	controller := defaultValue
	if alert.ReportingController != "" {
		controller = alert.ReportingController
	}

	revision := defaultValue
	if alert.Metadata.Revision != "" {
		revision = alert.Metadata.Revision
	}

	kind := defaultValue
	if alert.InvolvedObject.Kind != "" {
		kind = strings.ToLower(alert.InvolvedObject.Kind)
	}

	objectName := defaultValue
	if alert.InvolvedObject.Name != "" {
		objectName = alert.InvolvedObject.Name
	}

	message := noMessage
	if alert.Message != "" {
		message = alert.Message
	}

	// Hatékony string építés
	fmt.Fprintf(msgBuilder, "%s [%s]\n%s\n\nController: %s\nObject: %s/%s\nRevision: %s\n",
		reason, severity, message, controller, kind, objectName, revision)

	// Special handling for test mode
	if s.config.PushoverAPIToken == "test_api_token" {
		log.Println("Test mode: not sending to Pushover")
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(http.StatusOK)
		w.Write(responseOK)
		return
	}

	// Send to Pushover
	if err := s.sendToPushover(msgBuilder.String()); err != nil {
		log.Printf("Failed to send to Pushover: %v", err)
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(http.StatusInternalServerError)
		// Dinamikus hiba válasz, mert ezt nem tudjuk előre
		fmt.Fprintf(w, `{"error": "Failed to send to Pushover", "details": "%s"}`, err.Error())
		return
	}
	
	log.Printf("Successfully sent alert to Pushover for %s/%s", kind, objectName)
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(responseOK)
}

// sendToPushover sends a message to Pushover API
func (s *Server) sendToPushover(message string) error {
	data := url.Values{}
	data.Set("token", s.config.PushoverAPIToken)
	data.Set("user", s.config.PushoverUserKey)
	data.Set("message", message)
	data.Set("title", appTitle)

	req, err := http.NewRequestWithContext(context.Background(), "POST", pushoverURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", contentTypeForm)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Csak hiba esetén olvasunk response body-t
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("pushover API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Gyors discard
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func main() {
	// Simple health check mode for Docker HEALTHCHECK
	if len(os.Args) > 1 && os.Args[1] == "-health" {
		resp, err := http.Get("http://localhost:8080/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		resp.Body.Close()
		os.Exit(0)
	}

	// Load configuration from environment
	config := &Config{
		PushoverUserKey:  os.Getenv("PUSHOVER_USER_KEY"),
		PushoverAPIToken: os.Getenv("PUSHOVER_API_TOKEN"),
	}

	// Validate configuration
	if config.PushoverUserKey == "" || config.PushoverAPIToken == "" {
		log.Fatal("Pushover user key or API token is not configured, exiting app")
	}
	
	// Bearer token előre kiszámítása
	config.BearerToken = bearerPrefix + config.PushoverAPIToken

	// Create server
	server := NewServer(config)

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/", server.handleRoot)
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/webhook", server.handleWebhook)

	// Create HTTP server with timeouts
	httpServer := &http.Server{
		Addr:         serverPort,
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		MaxHeaderBytes: 1 << 20, // 1MB header limit
	}

	// Channel to listen for interrupt signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		log.Printf("Starting server on %s", serverPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-stop
	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
