package infrastructure

import (
	"net/http"
	"net/url"
	"time"
)

const (
	// ABIVersion is the current CLIProxyAPI plugin ABI version.
	ABIVersion uint32 = 1
	// SchemaVersion is the current plugin registration schema version.
	SchemaVersion uint32 = 1

	// MethodPluginRegister loads plugin metadata and capabilities.
	MethodPluginRegister = "plugin.register"
	// MethodPluginReconfigure reloads plugin-owned configuration.
	MethodPluginReconfigure = "plugin.reconfigure"
	// MethodModelRegister registers plugin-provided model metadata.
	MethodModelRegister = "model.register"
	// MethodModelRoute asks the plugin to route a matching model request.
	MethodModelRoute = "model.route"
	// MethodUsageHandle sends completed usage records to the plugin.
	MethodUsageHandle = "usage.handle"
	// MethodManagementRegister registers plugin-owned management routes.
	MethodManagementRegister = "management.register"
	// MethodManagementHandle forwards a management request to the plugin.
	MethodManagementHandle = "management.handle"
	// MethodExecutorIdentifier returns this plugin executor identifier.
	MethodExecutorIdentifier = "executor.identifier"
	// MethodExecutorExecute runs a non-streaming executor request.
	MethodExecutorExecute = "executor.execute"
	// MethodExecutorExecuteStream runs a streaming executor request.
	MethodExecutorExecuteStream = "executor.execute_stream"
	// MethodExecutorCountTokens counts tokens for an executor request.
	MethodExecutorCountTokens = "executor.count_tokens"

	// MethodHostHTTPDo executes one HTTP request through the host.
	MethodHostHTTPDo = "host.http.do"
	// MethodHostModelExecute executes one model request through the host.
	MethodHostModelExecute = "host.model.execute"
	// MethodHostLog writes one host log event.
	MethodHostLog = "host.log"
)

const (
	// ConfigFieldTypeString describes a string config value.
	ConfigFieldTypeString ConfigFieldType = "string"
	// ConfigFieldTypeEnum describes a constrained string config value.
	ConfigFieldTypeEnum ConfigFieldType = "enum"
	// ConfigFieldTypeObject describes an object config value.
	ConfigFieldTypeObject ConfigFieldType = "object"
)

// ConfigFieldType classifies plugin-owned configuration fields.
type ConfigFieldType string

// ConfigField describes one plugin-owned configuration field.
type ConfigField struct {
	Name        string
	Type        ConfigFieldType
	EnumValues  []string
	Description string
}

// Metadata describes plugin metadata returned during registration.
type Metadata struct {
	Name             string
	Version          string
	Author           string
	GitHubRepository string
	Logo             string
	ConfigFields     []ConfigField
}

// ModelInfo describes one model exposed by the plugin.
type ModelInfo struct {
	ID                         string
	Object                     string
	Created                    int64
	OwnedBy                    string
	Type                       string
	DisplayName                string
	Name                       string
	Version                    string
	Description                string
	InputTokenLimit            int64
	OutputTokenLimit           int64
	SupportedGenerationMethods []string
	ContextLength              int64
	MaxCompletionTokens        int64
	SupportedParameters        []string
	SupportedInputModalities   []string
	SupportedOutputModalities  []string
	UserDefined                bool
}

// ModelRegistrationResponse returns plugin-provided models to the host.
type ModelRegistrationResponse struct {
	Provider string
	Models   []ModelInfo
}

// ModelRouteRequest describes the original request offered to the router.
type ModelRouteRequest struct {
	Plugin             Metadata
	PluginID           string
	SourceFormat       string
	RequestedModel     string
	Stream             bool
	Headers            http.Header
	Query              url.Values
	Body               []byte
	Metadata           map[string]any
	AvailableProviders []string
}

// ModelRouteResponse returns a router decision to the host.
type ModelRouteResponse struct {
	Handled     bool
	TargetKind  string
	Target      string
	TargetModel string
	Reason      string
}

// HostHTTPRequest is the payload for host.http.do.
type HostHTTPRequest struct {
	Method  string      `json:"method,omitempty"`
	URL     string      `json:"url,omitempty"`
	Headers http.Header `json:"headers,omitempty"`
	Body    []byte      `json:"body,omitempty"`
}

// HostHTTPResponse is the response from host.http.do.
type HostHTTPResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// HostModelExecutionRequest is the payload for host.model.execute.
type HostModelExecutionRequest struct {
	EntryProtocol string      `json:"entry_protocol"`
	ExitProtocol  string      `json:"exit_protocol"`
	Model         string      `json:"model"`
	Stream        bool        `json:"stream"`
	Body          []byte      `json:"body"`
	Headers       http.Header `json:"headers"`
	Query         url.Values  `json:"query"`
	Alt           string      `json:"alt"`
}

// HostModelExecutionResponse is the response from host.model.execute.
type HostModelExecutionResponse struct {
	StatusCode int         `json:"status_code"`
	Headers    http.Header `json:"headers"`
	Body       []byte      `json:"body"`
}

// HostModelExecutionRPCRequest carries recursion protection metadata.
type HostModelExecutionRPCRequest struct {
	HostModelExecutionRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

// HostLogRequest is the payload for host.log.
type HostLogRequest struct {
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

// ManagementRegistrationResponse lists plugin-owned management routes.
type ManagementRegistrationResponse struct {
	Routes    []ManagementRoute
	Resources []ResourceRoute
}

// ManagementRoute describes one management route.
type ManagementRoute struct {
	Method      string
	Path        string
	Menu        string
	Description string
}

// ResourceRoute describes one browser resource route.
type ResourceRoute struct {
	Path        string
	Menu        string
	Description string
}

// ManagementRequest describes an authenticated management request.
type ManagementRequest struct {
	Method  string
	Path    string
	Headers http.Header
	Query   url.Values
	Body    []byte
}

// ManagementResponse describes a management response.
type ManagementResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// UsageRecord describes request usage metadata.
type UsageRecord struct {
	Provider        string
	ExecutorType    string
	Model           string
	Alias           string
	Source          string
	RequestedAt     time.Time
	Latency         time.Duration
	TTFT            time.Duration
	Failed          bool
	Failure         UsageFailure
	Detail          UsageDetail
	ResponseHeaders http.Header
}

// UsageFailure describes a failed request.
type UsageFailure struct {
	StatusCode int
	Body       string
}

// UsageDetail contains token counters.
type UsageDetail struct {
	InputTokens         int64
	OutputTokens        int64
	ReasoningTokens     int64
	CachedTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	TotalTokens         int64
}

// ExecutorRequest describes a direct plugin executor call.
type ExecutorRequest struct {
	AuthID          string
	AuthProvider    string
	Model           string
	Format          string
	Stream          bool
	Alt             string
	Headers         http.Header
	Query           url.Values
	OriginalRequest []byte
	SourceFormat    string
	Payload         []byte
	Metadata        map[string]any
}

// ExecutorResponse returns a non-streaming executor result.
type ExecutorResponse struct {
	Payload  []byte
	Headers  http.Header
	Metadata map[string]any
}

// ExecutorRPCRequest wraps an executor request with host callback metadata.
type ExecutorRPCRequest struct {
	ExecutorRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}
