package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
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
		if _, err := w.Write(types.ResponseRootError); err != nil {
			return
		}
	}
}

// CreateHealthHandler creates a handler for the health endpoint (pure function)
func CreateHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(types.ResponseHealthy); err != nil {
			return
		}
	}
}

// CreateWebhookHandler creates a webhook handler with dependencies
func CreateWebhookHandler(deps *HandlerDependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := server.RequestIDFromContext(r.Context())
		log := deps.Logger.With("request_id", requestID)

		if r.Method != http.MethodPost {
			log.Warn("invalid method", "method", r.Method, "remote_addr", r.RemoteAddr)
			writeJSONResponse(w, http.StatusMethodNotAllowed, types.ResponseMethodNotAllowed)
			return
		}

		// Check authorization (constant-time comparison to prevent timing attacks)
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(deps.Config.BearerToken)) != 1 {
			log.Warn("unauthorized request", "remote_addr", r.RemoteAddr)
			writeJSONResponse(w, http.StatusUnauthorized, types.ResponseUnauthorized)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, types.MaxBodySize)
		defer r.Body.Close()

		var alert types.FluxAlert
		decoder := json.NewDecoder(r.Body)

		if err := decoder.Decode(&alert); err != nil {
			log.Warn("failed to parse JSON", "error", err)
			writeJSONResponse(w, http.StatusBadRequest, types.ResponseInvalidJSON)
			return
		}

		if err := ValidateAlert(&alert); err != nil {
			log.Warn("invalid alert", "error", err)
			writeJSONResponse(w, http.StatusBadRequest, types.ResponseInvalidJSON)
			return
		}

		message := deps.MessageBuilder(&alert)

		if deps.Config.PushoverAPIToken == "test_api_token" {
			log.Info("test mode: skipping Pushover send")
			writeJSONResponse(w, http.StatusOK, types.ResponseOK)
			return
		}

		pushoverMsg := CreatePushoverMessage(deps.Config, message)
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if err := deps.PushoverClient.SendMessage(ctx, pushoverMsg); err != nil {
			log.Error("failed to send to Pushover", "error", err)
			writeJSONResponse(w, http.StatusBadGateway, types.ResponseUpstreamError)
			return
		}

		info := ExtractAlertInfo(&alert)
		log.Info("alert sent to Pushover", "kind", info["kind"], "name", info["name"])
		writeJSONResponse(w, http.StatusOK, types.ResponseOK)
	}
}

// writeJSONResponse writes a JSON response with proper headers
func writeJSONResponse(w http.ResponseWriter, statusCode int, body []byte) {
	w.Header().Set("Content-Type", types.ContentTypeJSON)
	w.WriteHeader(statusCode)
	if _, err := w.Write(body); err != nil {
		return
	}
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
	httpClient := pushover.CreateOptimizedHTTPClient(10 * time.Second)
	pushoverClient := pushover.NewPushoverClient(httpClient, cfg.PushoverURL)

	deps := &HandlerDependencies{
		Config:         cfg,
		PushoverClient: pushoverClient,
		Logger:         logger,
		MessageBuilder: BuildPushoverMessage,
	}

	return deps, nil
}
