package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	pushoverURL = "https://api.pushover.net/1/messages.json"
	serverPort  = ":8080"
	readTimeout = 10 * time.Second
	writeTimeout = 10 * time.Second
	shutdownTimeout = 30 * time.Second
)

// Config holds application configuration
type Config struct {
	PushoverUserKey  string
	PushoverAPIToken string
}

// FluxAlert represents the incoming webhook payload from Flux
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

// NewServer creates a new server instance
func NewServer(config *Config) *Server {
	return &Server{
		config: config,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}
// handleRoot handles requests to the root path
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Requests need to be made to /webhook", http.StatusBadRequest)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("healthy"))
}

// handleWebhook handles incoming webhook requests from Flux
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// Verify Authorization header
	authHeader := r.Header.Get("Authorization")
	expectedAuth := fmt.Sprintf("Bearer %s", s.config.PushoverAPIToken)
	
	if authHeader != expectedAuth {
		log.Printf("Unauthorized request from %s", r.RemoteAddr)
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Parse JSON payload
	var alert FluxAlert
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		http.Error(w, `{"error": "Failed to read request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	if err := json.Unmarshal(body, &alert); err != nil {
		log.Printf("Failed to parse JSON: %v", err)
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Build Pushover message
	severity := strings.ToUpper(alert.Severity)
	if severity == "" {
		severity = "INFO"
	}

	reason := alert.Reason
	if reason == "" {
		reason = "Unknown"
	}

	controller := alert.ReportingController
	if controller == "" {
		controller = "Unknown"
	}

	revision := alert.Metadata.Revision
	if revision == "" {
		revision = "Unknown"
	}

	kind := alert.InvolvedObject.Kind
	if kind == "" {
		kind = "Unknown"
	}
	objectName := alert.InvolvedObject.Name
	if objectName == "" {
		objectName = "Unknown"
	}

	message := alert.Message
	if message == "" {
		message = "No Message"
	}

	pushoverMessage := fmt.Sprintf(
		"%s [%s]\n%s\n\nController: %s\nObject: %s/%s\nRevision: %s\n",
		reason, severity, message, controller, 
		strings.ToLower(kind), objectName, revision,
	)

	// Special handling for test mode
	if s.config.PushoverAPIToken == "test_api_token" {
		log.Println("Test mode: not sending to Pushover")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
		return
	}

	// Send to Pushover
	if err := s.sendToPushover(pushoverMessage); err != nil {
		log.Printf("Failed to send to Pushover: %v", err)
		http.Error(w, fmt.Sprintf(`{"error": "Failed to send to Pushover", "details": "%s"}`, err.Error()), 
			http.StatusInternalServerError)
		return
	}
	log.Printf("Successfully sent alert to Pushover for %s/%s", kind, objectName)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "ok"}`))
}

// sendToPushover sends a message to Pushover API
func (s *Server) sendToPushover(message string) error {
	data := url.Values{}
	data.Set("token", s.config.PushoverAPIToken)
	data.Set("user", s.config.PushoverUserKey)
	data.Set("message", message)
	data.Set("title", "FluxCD")

	req, err := http.NewRequest("POST", pushoverURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pushover API returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
func main() {
	// Load configuration from environment
	config := &Config{
		PushoverUserKey:  os.Getenv("PUSHOVER_USER_KEY"),
		PushoverAPIToken: os.Getenv("PUSHOVER_API_TOKEN"),
	}

	// Validate configuration
	if config.PushoverUserKey == "" || config.PushoverAPIToken == "" {
		log.Fatal("Pushover user key or API token is not configured, exiting app")
	}

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