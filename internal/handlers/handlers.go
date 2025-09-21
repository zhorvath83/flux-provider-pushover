package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/pushover"
	"github.com/zhorvath83/flux-provider-pushover/internal/server"
	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

// PushoverSender interface for sending messages
type PushoverSender interface {
	SendMessage(ctx context.Context, msg *types.PushoverMessage) error
}

// HandlerDependencies contains all dependencies for handlers
type HandlerDependencies struct {
	Config         *config.Config
	PushoverClient PushoverSender
	Logger         server.Logger
	MessageBuilder MessageBuilder
}

// CreateRootHandler creates a handler for the root endpoint (pure function)
func CreateRootHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(types.ResponseRootError)
	}
}

// CreateHealthHandler creates a handler for the health endpoint (pure function)
func CreateHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(types.ResponseHealthy)
	}
}

// CreateWebhookHandler creates a webhook handler with dependencies
func CreateWebhookHandler(deps *HandlerDependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Handle OPTIONS requests for CORS
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Only accept POST requests
		if r.Method != http.MethodPost {
			deps.Logger.Printf("Invalid method %s from %s", r.Method, r.RemoteAddr)
			writeJSONResponse(w, http.StatusMethodNotAllowed, types.ResponseMethodNotAllowed)
			return
		}

		// Check authorization
		if r.Header.Get("Authorization") != deps.Config.BearerToken {
			deps.Logger.Printf("Unauthorized request from %s", r.RemoteAddr)
			writeJSONResponse(w, http.StatusUnauthorized, types.ResponseUnauthorized)
			return
		}

		// Limit request body size
		r.Body = http.MaxBytesReader(w, r.Body, types.MaxBodySize)
		defer r.Body.Close()

		// Parse JSON payload
		var alert types.FluxAlert
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&alert); err != nil {
			deps.Logger.Printf("Failed to parse JSON: %v", err)
			writeJSONResponse(w, http.StatusBadRequest, types.ResponseInvalidJSON)
			return
		}

		// Validate alert
		if err := ValidateAlert(&alert); err != nil {
			deps.Logger.Printf("Invalid alert: %v", err)
			writeJSONResponse(w, http.StatusBadRequest, types.ResponseInvalidJSON)
			return
		}

		// Build message
		message := deps.MessageBuilder(&alert)

		// Special handling for test mode
		if deps.Config.PushoverAPIToken == "test_api_token" {
			deps.Logger.Println("Test mode: not sending to Pushover")
			writeJSONResponse(w, http.StatusOK, types.ResponseOK)
			return
		}

		// Create and send Pushover message
		pushoverMsg := CreatePushoverMessage(deps.Config, message)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := deps.PushoverClient.SendMessage(ctx, pushoverMsg); err != nil {
			deps.Logger.Printf("Failed to send to Pushover: %v", err)
			errorResponse := fmt.Sprintf(`{"error": "Failed to send to Pushover", "details": "%s"}`, err.Error())
			writeJSONResponse(w, http.StatusInternalServerError, []byte(errorResponse))
			return
		}

		// Log success
		info := ExtractAlertInfo(&alert)
		deps.Logger.Printf("Successfully sent alert to Pushover for %s/%s", info["kind"], info["name"])
		writeJSONResponse(w, http.StatusOK, types.ResponseOK)
	}
}

// writeJSONResponse writes a JSON response with proper headers
func writeJSONResponse(w http.ResponseWriter, statusCode int, body []byte) {
	w.Header().Set("Content-Type", types.ContentTypeJSON)
	w.WriteHeader(statusCode)
	w.Write(body)
}

// CreateRouter creates the HTTP router with all endpoints
func CreateRouter(deps *HandlerDependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", CreateRootHandler())
	mux.HandleFunc("/health", CreateHealthHandler())
	mux.HandleFunc("/webhook", CreateWebhookHandler(deps))
	return mux
}

// CreateServerDependencies creates all server dependencies
func CreateServerDependencies(cfg *config.Config, logger server.Logger) (*HandlerDependencies, error) {
	// Create HTTP client
	httpClient := pushover.CreateOptimizedHTTPClient(10 * time.Second)

	// Create Pushover client
	pushoverClient := pushover.NewPushoverClient(httpClient, cfg.PushoverURL)

	// Create dependencies
	deps := &HandlerDependencies{
		Config:         cfg,
		PushoverClient: pushoverClient,
		Logger:         logger,
		MessageBuilder: BuildPushoverMessage,
	}

	return deps, nil
}
