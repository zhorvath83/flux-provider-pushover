package types

// ObjectReference represents a Kubernetes object reference
type ObjectReference struct {
	Kind            string `json:"kind"`
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	UID             string `json:"uid"`
	APIVersion      string `json:"apiVersion"`
	ResourceVersion string `json:"resourceVersion"`
}

// FluxAlert represents an alert from FluxCD
type FluxAlert struct {
	InvolvedObject      ObjectReference   `json:"involvedObject"`
	Severity            string            `json:"severity"`
	Timestamp           string            `json:"timestamp"`
	Message             string            `json:"message"`
	Reason              string            `json:"reason"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	ReportingController string            `json:"reportingController"`
	ReportingInstance   string            `json:"reportingInstance,omitempty"`
}

// PushoverMessage represents a message to be sent to Pushover
type PushoverMessage struct {
	Token   string
	User    string
	Title   string
	Message string
}

// Constants for default values
const (
	DefaultSeverity = "INFO"
	DefaultValue    = "Unknown"
	NoMessage       = "No Message"
	AppTitle        = "FluxCD"

	// HTTP related constants
	ContentTypeJSON = "application/json"
	ContentTypeForm = "application/x-www-form-urlencoded"
	BearerPrefix    = "Bearer "

	// Server constants
	ServerPort      = ":8080"
	ReadTimeout     = 10      // seconds
	WriteTimeout    = 10      // seconds
	ShutdownTimeout = 30      // seconds
	MaxBodySize     = 1 << 20 // 1MB
	MaxHeaderSize   = 1 << 13 // 8KB
)

// Pre-defined JSON responses
var (
	ResponseOK               = []byte(`{"status": "ok"}`)
	ResponseUnauthorized     = []byte(`{"error": "Unauthorized"}`)
	ResponseInvalidJSON      = []byte(`{"error": "Invalid JSON"}`)
	ResponseMethodNotAllowed = []byte(`{"error": "Method not allowed"}`)
	ResponseUpstreamError    = []byte(`{"error": "Upstream service unavailable"}`)
	ResponseRateLimitError   = []byte(`{"error": "Rate limit exceeded"}`)
	ResponseRootError        = []byte("Requests need to be made to /webhook")
	ResponseHealthy          = []byte("healthy")
)
