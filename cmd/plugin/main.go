package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stored_host;

static void store_host_api(const cliproxy_host_api* host) {
	stored_host = host;
}

static int call_host_api(const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	if (stored_host == NULL || stored_host->call == NULL) {
		return 1;
	}
	return stored_host->call(stored_host->host_ctx, method, request, request_len, response);
}

static void free_host_buffer(void* ptr, size_t len) {
	if (stored_host != NULL && stored_host->free_buffer != NULL && ptr != NULL) {
		stored_host->free_buffer(ptr, len);
	}
}
*/
import "C"

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/vfeitoza/cli-smart-router/internal/application"
	"github.com/vfeitoza/cli-smart-router/internal/domain"
	"github.com/vfeitoza/cli-smart-router/internal/infrastructure"
)

const pluginIdentifier = "smart-model-router"

var (
	configStore  = infrastructure.NewConfigStore()
	usageLearner application.UsageLearner
	runtimeState = infrastructure.NewRuntimeState()
)

type registration struct {
	SchemaVersion uint32                  `json:"schema_version"`
	Metadata      infrastructure.Metadata `json:"metadata"`
	Capabilities  registrationCapability  `json:"capabilities"`
}

type registrationCapability struct {
	ModelRouter           bool     `json:"model_router"`
	ModelRegistrar        bool     `json:"model_registrar"`
	UsagePlugin           bool     `json:"usage_plugin"`
	ManagementAPI         bool     `json:"management_api"`
	Executor              bool     `json:"executor,omitempty"`
	ExecutorModelScope    string   `json:"executor_model_scope,omitempty"`
	ExecutorInputFormats  []string `json:"executor_input_formats,omitempty"`
	ExecutorOutputFormats []string `json:"executor_output_formats,omitempty"`
}

type rpcModelRouteRequest struct {
	infrastructure.ModelRouteRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type classifierTrace struct {
	Enabled  bool   `json:"enabled"`
	Used     bool   `json:"used"`
	Model    string `json:"model,omitempty"`
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
	plugin.abi_version = C.uint32_t(infrastructure.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, infrastructure.ErrorEnvelope("invalid_method", "method is required"))
		return 1
	}
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, errHandle := handleMethod(C.GoString(method), requestBytes)
	if errHandle != nil {
		writeResponse(response, infrastructure.ErrorEnvelope("plugin_error", errHandle.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case infrastructure.MethodPluginRegister, infrastructure.MethodPluginReconfigure:
		cfg, errParse := infrastructure.ParseConfig(request)
		if errParse != nil {
			return nil, errParse
		}
		configStore.Store(cfg)
		_ = runtimeState.LoadFromFile(cfg.StatePath)
		refreshExternalState(cfg)
		return infrastructure.OKEnvelope(pluginRegistration())
	case infrastructure.MethodModelRegister:
		registrar := application.Registrar{Config: configStore.Load()}
		return infrastructure.OKEnvelope(registrar.Register())
	case infrastructure.MethodModelRoute:
		return routeModel(request)
	case infrastructure.MethodUsageHandle:
		return handleUsage(request)
	case infrastructure.MethodManagementRegister:
		return managementRegister()
	case infrastructure.MethodManagementHandle:
		return managementHandle(request)
	case infrastructure.MethodExecutorIdentifier:
		return infrastructure.OKEnvelope(map[string]string{"identifier": pluginIdentifier})
	case infrastructure.MethodExecutorExecute:
		return executeWithFallback(request)
	case infrastructure.MethodExecutorExecuteStream:
		return infrastructure.ErrorEnvelope("unsupported_stream", "streaming executor fallback is not implemented"), nil
	case infrastructure.MethodExecutorCountTokens:
		return infrastructure.OKEnvelope(infrastructure.ExecutorResponse{Payload: []byte(`{"input_tokens":0}`)})
	default:
		return infrastructure.ErrorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func routeModel(raw []byte) ([]byte, error) {
	var req rpcModelRouteRequest
	if errUnmarshal := json.Unmarshal(raw, &req); errUnmarshal != nil {
		return nil, errUnmarshal
	}
	cfg := configStore.Load()
	if strings.TrimSpace(req.RequestedModel) != cfg.VirtualModel {
		return infrastructure.OKEnvelope(infrastructure.ModelRouteResponse{Handled: false})
	}
	runtimeState.Inc("router_requests_total")
	refreshExternalState(cfg)
	if sessionEntry, ok := sessionRoute(cfg, req.ModelRouteRequest); ok {
		logRouteDecision(cfg, req.ModelRouteRequest, sessionEntry, "session", nil)
		return routeResponse(sessionEntry, cfg, req.ModelRouteRequest)
	}
	if cachedEntry, ok := cachedRoute(cfg, req.ModelRouteRequest); ok {
		runtimeState.Inc("router_cache_hits")
		logRouteDecision(cfg, req.ModelRouteRequest, cachedEntry, "cache", nil)
		return routeResponse(cachedEntry, cfg, req.ModelRouteRequest)
	}
	var trace *classifierTrace
	var entry infrastructure.RouteCacheEntry
	selected := false

	router := application.Router{Config: cfg}
	localDecision := router.Route(req.ModelRouteRequest)

	switch cfg.Strategy {
	case "llm":
		// llm always tries the classifier first; deterministic fallback below
		// covers classifier failure. This strategy intentionally pays classifier
		// latency/cost on every uncached request in exchange for a classifier
		// opinion on every decision.
		classified, ok, classifier := classifyRoute(cfg, req.ModelRouteRequest)
		trace = &classifier
		if ok {
			classified = applyPreferenceTiebreak(cfg, req.ModelRouteRequest, classified)
			entry = classified
			selected = true
		}
	case "hybrid":
		// hybrid is local-first: local capability-aware scoring runs first (no
		// host call), and the classifier is only consulted when the local
		// decision is not confident (the prompt matched no known capability
		// signal). This avoids paying classifier latency/cost for prompts the
		// configured `models.capabilities` already answer clearly.
		if localDecision.Handled && localDecision.Confident {
			entry = routeCacheEntryFromDecision(localDecision, "local_confident")
			selected = true
			runtimeState.Inc("router_local_confident")
		} else {
			classified, ok, classifier := classifyRoute(cfg, req.ModelRouteRequest)
			trace = &classifier
			if ok {
				classified = applyPreferenceTiebreak(cfg, req.ModelRouteRequest, classified)
				entry = classified
				selected = true
			}
		}
	}
	if !selected {
		if !localDecision.Handled {
			return infrastructure.OKEnvelope(infrastructure.ModelRouteResponse{Handled: false, Reason: localDecision.Reason})
		}
		entry = routeCacheEntryFromDecision(localDecision, "")
	}
	storeRoute(cfg, req.ModelRouteRequest, entry)
	logRouteDecision(cfg, req.ModelRouteRequest, entry, "selected", trace)
	return routeResponse(entry, cfg, req.ModelRouteRequest)
}

// routeCacheEntryFromDecision converts a domain.RouteDecision into a cacheable
// route entry, optionally prefixing the reason with a source tag such as
// "local_confident" so debug logs and cache entries distinguish local-first
// hybrid decisions from plain deterministic fallback.
func routeCacheEntryFromDecision(decision domain.RouteDecision, sourceTag string) infrastructure.RouteCacheEntry {
	reason := decision.Reason
	if sourceTag != "" {
		reason = sourceTag + " " + reason
	}
	return infrastructure.RouteCacheEntry{Provider: decision.TargetProvider, Model: decision.TargetModel, Reason: reason, CreatedAt: time.Now()}
}

func logRouteDecision(cfg domain.Config, req infrastructure.ModelRouteRequest, entry infrastructure.RouteCacheEntry, source string, classifier *classifierTrace) {
	if !cfg.Debug.Enabled || strings.TrimSpace(cfg.Debug.LogPath) == "" {
		return
	}
	record := map[string]any{
		"time":            time.Now().Format(time.RFC3339Nano),
		"source":          source,
		"virtual_model":   cfg.VirtualModel,
		"source_format":   req.SourceFormat,
		"stream":          req.Stream,
		"strategy":        cfg.Strategy,
		"preference":      cfg.Preference,
		"target_provider": entry.Provider,
		"target_model":    entry.Model,
		"reason":          entry.Reason,
	}
	if classifier != nil {
		record["classifier"] = classifier
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return
	}
	f, err := os.OpenFile(cfg.Debug.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(raw, '\n'))
}

func routeResponse(entry infrastructure.RouteCacheEntry, cfg domain.Config, req infrastructure.ModelRouteRequest) ([]byte, error) {
	if cfg.ExecutorFallback.Enabled && !req.Stream {
		return infrastructure.OKEnvelope(infrastructure.ModelRouteResponse{
			Handled:    true,
			TargetKind: "self",
			Reason:     entry.Reason + " executor_fallback",
		})
	}
	return infrastructure.OKEnvelope(infrastructure.ModelRouteResponse{
		Handled:     true,
		TargetKind:  "provider",
		Target:      entry.Provider,
		TargetModel: entry.Model,
		Reason:      entry.Reason,
	})
}

func refreshExternalState(cfg domain.Config) {
	snapshot := runtimeState.Snapshot()
	if cfg.Catalog.BaseURL != "" && shouldRefresh(snapshot.Catalog.FetchedAt, cfg.Catalog.RefreshInterval, 10*time.Minute) {
		models, err := fetchModelCatalog(cfg)
		if err != nil {
			runtimeState.SetCatalog(nil, err.Error())
		} else {
			runtimeState.SetCatalog(models, "")
		}
	}
	if cfg.Pricing.Enabled && cfg.Pricing.URL != "" && shouldRefresh(snapshot.Pricing.FetchedAt, cfg.Pricing.RefreshInterval, 6*time.Hour) {
		body, err := fetchURL(cfg.Pricing.URL, "")
		if err != nil {
			runtimeState.SetPricing(0, err.Error())
		} else {
			runtimeState.SetPricing(len(body), "")
		}
	}
}

func shouldRefresh(last time.Time, configured string, fallback time.Duration) bool {
	if last.IsZero() {
		return true
	}
	interval := fallback
	if parsed, err := time.ParseDuration(configured); err == nil && parsed > 0 {
		interval = parsed
	}
	return time.Since(last) >= interval
}

func fetchModelCatalog(cfg domain.Config) ([]string, error) {
	body, err := fetchURL(cfg.Catalog.BaseURL+"/v1/models", cfg.Catalog.APIKey)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if item.ID != "" && (cfg.Catalog.IncludeRouterModel || item.ID != cfg.VirtualModel) {
			models = append(models, item.ID)
		}
	}
	return models, nil
}

func fetchURL(url string, apiKey string) ([]byte, error) {
	headers := http.Header{}
	if strings.TrimSpace(apiKey) != "" {
		headers.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	resp, err := callHost[infrastructure.HostHTTPResponse](infrastructure.MethodHostHTTPDo, infrastructure.HostHTTPRequest{Method: http.MethodGet, URL: url, Headers: headers})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func sessionRoute(cfg domain.Config, req infrastructure.ModelRouteRequest) (infrastructure.RouteCacheEntry, bool) {
	if !cfg.Routing.KeepSameModelPerSession {
		return infrastructure.RouteCacheEntry{}, false
	}
	sessionID := metadataString(req.Metadata, "execution_session_id")
	return runtimeState.GetSessionRoute(sessionID)
}

func cachedRoute(cfg domain.Config, req infrastructure.ModelRouteRequest) (infrastructure.RouteCacheEntry, bool) {
	if !cfg.Cache.Enabled {
		return infrastructure.RouteCacheEntry{}, false
	}
	return runtimeState.GetCachedRoute(routeCacheKey(req), cacheTTL(cfg))
}

func storeRoute(cfg domain.Config, req infrastructure.ModelRouteRequest, entry infrastructure.RouteCacheEntry) {
	if cfg.Cache.Enabled {
		runtimeState.SetCachedRoute(routeCacheKey(req), entry, cfg.Cache.MaxEntries)
	}
	if cfg.Routing.KeepSameModelPerSession {
		if sessionID := metadataString(req.Metadata, "execution_session_id"); sessionID != "" {
			runtimeState.SetSessionRoute(sessionID, entry)
		}
	}
	runtimeState.SaveThrottled(cfg.StatePath, 30*time.Second)
}

// routeCacheKey hashes the semantic prompt (last user message) so identical prompts
// map to the same decision regardless of surrounding conversation history.
func routeCacheKey(req infrastructure.ModelRouteRequest) string {
	prompt := infrastructure.ExtractUserPrompt(req.Body)
	h := sha256.New()
	h.Write([]byte(req.RequestedModel))
	h.Write([]byte{0})
	h.Write([]byte(strings.TrimSpace(prompt)))
	return hex.EncodeToString(h.Sum(nil))
}

// cacheTTL parses the configured cache TTL; zero means entries never expire by age.
func cacheTTL(cfg domain.Config) time.Duration {
	if d, err := time.ParseDuration(cfg.Cache.TTL); err == nil && d > 0 {
		return d
	}
	return 0
}

func classifyRoute(cfg domain.Config, req infrastructure.ModelRouteRequest) (infrastructure.RouteCacheEntry, bool, classifierTrace) {
	trace := classifierTrace{Enabled: cfg.Classifier.Enabled}
	if !cfg.Classifier.Enabled || len(cfg.Classifier.Models) == 0 {
		if len(cfg.Classifier.Models) == 0 {
			trace.Error = "classifier has no configured models"
		}
		return infrastructure.RouteCacheEntry{}, false, trace
	}
	candidates := configuredCandidateSet(cfg)
	maxAttempts := cfg.Classifier.MaxAttempts
	if maxAttempts <= 0 || maxAttempts > len(cfg.Classifier.Models) {
		maxAttempts = len(cfg.Classifier.Models)
	}
	for i := 0; i < maxAttempts; i++ {
		classifier := cfg.Classifier.Models[i]
		if classifier.Model == "" {
			continue
		}
		trace.Model = classifier.Model
		runtimeState.Inc("router_classifier_calls")
		body := classifierRequestBody(classifier.Model, cfg, req)
		resp, err := callHost[infrastructure.HostModelExecutionResponse](infrastructure.MethodHostModelExecute, infrastructure.HostModelExecutionRequest{EntryProtocol: "openai", ExitProtocol: "openai", Model: classifier.Model, Stream: false, Body: body})
		if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			runtimeState.Inc("router_classifier_failures")
			if err != nil {
				trace.Error = err.Error()
			} else {
				trace.Error = fmt.Sprintf("classifier status %d", resp.StatusCode)
			}
			continue
		}
		content := classifierContent(resp.Body)
		trace.Response = truncateLogString(string(content), 2000)
		var parsed struct {
			SelectedModel string  `json:"selected_model"`
			Confidence    float64 `json:"confidence"`
			Reason        string  `json:"reason"`
		}
		jsonBlob := extractJSONObject(content)
		if jsonBlob == nil || json.Unmarshal(jsonBlob, &parsed) != nil || parsed.SelectedModel == "" {
			runtimeState.Inc("router_classifier_failures")
			trace.Error = "classifier returned invalid JSON"
			continue
		}
		candidate, ok := candidates[parsed.SelectedModel]
		if !ok || !providerAvailable(candidate.Provider, req.AvailableProviders) {
			runtimeState.Inc("router_classifier_failures")
			trace.Error = "classifier selected unavailable model: " + parsed.SelectedModel
			continue
		}
		trace.Used = true
		trace.Error = ""
		return infrastructure.RouteCacheEntry{Provider: candidate.Provider, Model: candidate.Model, Reason: "classifier:" + parsed.Reason, CreatedAt: time.Now()}, true, trace
	}
	if trace.Error == "" {
		trace.Error = "classifier attempts exhausted"
	}
	return infrastructure.RouteCacheEntry{}, false, trace
}

func truncateLogString(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

// preferenceInstruction returns a bias phrase for the classifier system prompt.
func preferenceInstruction(preference string) string {
	switch preference {
	case domain.PreferenceCost:
		return "When two models could both handle the request, always choose the cheaper one."
	case domain.PreferenceQuality:
		return "When two models could both handle the request, choose the higher-quality one."
	default:
		return "When two models could both handle the request, balance cost and quality."
	}
}

// classifierRequestBody builds an isolated classification prompt so the classifier
// selects a model id instead of answering the user's original request.
func classifierRequestBody(classifierModel string, cfg domain.Config, req infrastructure.ModelRouteRequest) []byte {
	var catalog strings.Builder
	for _, candidate := range cfg.Models {
		if candidate.Model == "" {
			continue
		}
		fmt.Fprintf(&catalog, "- id=%s provider=%s cost=%s quality=%s capabilities=%s\n",
			candidate.Model, candidate.Provider, candidate.Cost, candidate.Quality, strings.Join(candidate.Capabilities, ","))
	}
	system := "You are a routing classifier. Pick the single best model id for the user request " +
		"from the catalog. Prefer the cheapest model that can handle the request well: simple/short " +
		"tasks (math, classification, summaries) go to low-cost models; complex coding, architecture or " +
		"deep reasoning go to high-quality models. " + preferenceInstruction(cfg.Preference) +
		" Respond with ONLY a compact JSON object and nothing " +
		"else: {\"selected_model\":\"<id>\",\"confidence\":<0-1>,\"reason\":\"<short>\"}."
	userPrompt := infrastructure.ExtractUserPrompt(req.Body)
	userContent := fmt.Sprintf("Model catalog:\n%s\nUser request:\n%s", catalog.String(), truncateLogString(userPrompt, 4000))
	payload := map[string]any{
		"model":       classifierModel,
		"stream":      false,
		"temperature": 0,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": userContent},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

// extractJSONObject returns the first balanced JSON object found in the content,
// tolerating classifier responses that wrap JSON in prose or code fences.
func extractJSONObject(content []byte) []byte {
	start := bytes.IndexByte(content, '{')
	if start < 0 {
		return nil
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(content); i++ {
		c := content[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return content[start : i+1]
			}
		}
	}
	return nil
}

func classifierContent(body []byte) []byte {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		return body
	}
	var openAI struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &openAI); err == nil && len(openAI.Choices) > 0 {
		content := strings.TrimSpace(openAI.Choices[0].Message.Content)
		if content != "" {
			return []byte(content)
		}
	}
	return body
}

// applyPreferenceTiebreak promotes the classifier pick toward the configured preference
// among equivalent-tier candidates, then updates the reason for auditability.
func applyPreferenceTiebreak(cfg domain.Config, req infrastructure.ModelRouteRequest, entry infrastructure.RouteCacheEntry) infrastructure.RouteCacheEntry {
	if cfg.Preference == domain.PreferenceBalanced {
		return entry
	}
	chosen, ok := configuredCandidateSet(cfg)[entry.Model]
	if !ok {
		return entry
	}
	candidates := make([]domain.Candidate, 0, len(cfg.Models))
	for _, item := range cfg.Models {
		candidates = append(candidates, domain.CandidateFromConfig(item))
	}
	promoted := domain.ApplyPreferenceTiebreak(domain.CandidateFromConfig(chosen), candidates, req.AvailableProviders, cfg.Preference)
	if promoted.Model == entry.Model {
		return entry
	}
	entry.Provider = promoted.Provider
	entry.Model = promoted.Model
	entry.Reason = fmt.Sprintf("%s preference_tiebreak:%s->%s", entry.Reason, chosen.Model, promoted.Model)
	return entry
}

func configuredCandidateSet(cfg domain.Config) map[string]domain.CandidateConfig {
	out := make(map[string]domain.CandidateConfig, len(cfg.Models))
	for _, candidate := range cfg.Models {
		if candidate.Model != "" && candidate.Provider != "" {
			out[candidate.Model] = candidate
		}
	}
	return out
}

func providerAvailable(provider string, available []string) bool {
	if len(available) == 0 {
		return true
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	for _, item := range available {
		if strings.ToLower(strings.TrimSpace(item)) == provider {
			return true
		}
	}
	return false
}

func metadataString(meta map[string]any, key string) string {
	if len(meta) == 0 {
		return ""
	}
	value, ok := meta[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func handleUsage(raw []byte) ([]byte, error) {
	var record infrastructure.UsageRecord
	if len(raw) > 0 {
		if errUnmarshal := json.Unmarshal(raw, &record); errUnmarshal != nil {
			return nil, errUnmarshal
		}
	}
	usageLearner.Record(record)
	runtimeState.RecordUsage(record)
	_ = runtimeState.SaveToFile(configStore.Load().StatePath)
	return infrastructure.OKEnvelope(map[string]bool{"handled": true})
}

func executeWithFallback(raw []byte) ([]byte, error) {
	var req infrastructure.ExecutorRPCRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	cfg := configStore.Load()
	maxAttempts := cfg.ExecutorFallback.MaxAttempts
	if maxAttempts <= 0 || maxAttempts > len(cfg.Models) {
		maxAttempts = len(cfg.Models)
	}
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		candidate := cfg.Models[i]
		if candidate.Model == "" {
			continue
		}
		requestBody := req.OriginalRequest
		if len(requestBody) == 0 {
			requestBody = req.Payload
		}
		resp, err := callHost[infrastructure.HostModelExecutionResponse](infrastructure.MethodHostModelExecute, infrastructure.HostModelExecutionRPCRequest{
			HostModelExecutionRequest: infrastructure.HostModelExecutionRequest{
				EntryProtocol: req.SourceFormat,
				ExitProtocol:  req.SourceFormat,
				Model:         candidate.Model,
				Stream:        false,
				Body:          requestBody,
				Headers:       req.Headers,
				Query:         req.Query,
				Alt:           req.Alt,
			},
			HostCallbackID: req.HostCallbackID,
		})
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return infrastructure.OKEnvelope(infrastructure.ExecutorResponse{Payload: resp.Body, Headers: resp.Headers})
		}
		lastErr = fmt.Errorf("candidate %s returned status %d", candidate.Model, resp.StatusCode)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no executor fallback candidate available")
	}
	return infrastructure.ErrorEnvelope("executor_fallback_failed", lastErr.Error()), nil
}

func managementRegister() ([]byte, error) {
	return infrastructure.OKEnvelope(infrastructure.ManagementRegistrationResponse{
		Routes: []infrastructure.ManagementRoute{{
			Method:      http.MethodGet,
			Path:        "/plugins/smart-model-router/status",
			Description: "Shows smart-model-router status.",
		}},
	})
}

func managementHandle(raw []byte) ([]byte, error) {
	var req infrastructure.ManagementRequest
	if len(raw) > 0 {
		if errUnmarshal := json.Unmarshal(raw, &req); errUnmarshal != nil {
			return nil, errUnmarshal
		}
	}
	if req.Method != http.MethodGet || !strings.HasSuffix(req.Path, "/plugins/smart-model-router/status") {
		return infrastructure.OKEnvelope(infrastructure.ManagementResponse{
			StatusCode: http.StatusNotFound,
			Headers:    http.Header{"Content-Type": []string{"application/json"}},
			Body:       []byte(`{"error":"not_found"}`),
		})
	}
	body, errMarshal := json.Marshal(map[string]any{
		"plugin":        pluginIdentifier,
		"virtual_model": configStore.Load().VirtualModel,
		"strategy":      configStore.Load().Strategy,
		"usage":         usageLearner.Snapshot(),
		"state":         runtimeState.Snapshot(),
	})
	if errMarshal != nil {
		return nil, errMarshal
	}
	return infrastructure.OKEnvelope(infrastructure.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{"Content-Type": []string{"application/json"}},
		Body:       body,
	})
}

func pluginRegistration() registration {
	cfg := configStore.Load()
	capabilities := registrationCapability{
		ModelRouter:    true,
		ModelRegistrar: true,
		UsagePlugin:    true,
		ManagementAPI:  true,
	}
	if cfg.ExecutorFallback.Enabled {
		capabilities.Executor = true
		capabilities.ExecutorModelScope = "static"
		capabilities.ExecutorInputFormats = []string{"openai", "claude", "gemini"}
		capabilities.ExecutorOutputFormats = []string{"openai", "claude", "gemini"}
	}
	return registration{
		SchemaVersion: infrastructure.SchemaVersion,
		Metadata: infrastructure.Metadata{
			Name:             pluginIdentifier,
			Version:          "0.1.1",
			Author:           "Victor Feitoza",
			GitHubRepository: "https://github.com/vfeitoza/cli-smart-router",
			ConfigFields: []infrastructure.ConfigField{
				{Name: "virtual_model", Type: infrastructure.ConfigFieldTypeString, Description: "Virtual model name intercepted by the router. Default: router:auto."},
				{Name: "strategy", Type: infrastructure.ConfigFieldTypeEnum, EnumValues: []string{"capability", "benchmark", "llm", "hybrid"}, Description: "Routing strategy. V1 uses deterministic capability routing."},
				{Name: "debug", Type: infrastructure.ConfigFieldTypeObject, Description: "Optional non-sensitive route decision JSONL logging settings."},
				{Name: "catalog", Type: infrastructure.ConfigFieldTypeObject, Description: "Catalog refresh settings for CLIProxyAPI /v1/models."},
				{Name: "pricing", Type: infrastructure.ConfigFieldTypeObject, Description: "Optional external pricing refresh settings."},
				{Name: "cache", Type: infrastructure.ConfigFieldTypeObject, Description: "Route decision cache settings."},
				{Name: "executor_fallback", Type: infrastructure.ConfigFieldTypeObject, Description: "Optional non-streaming same-request fallback executor settings."},
				{Name: "classifier", Type: infrastructure.ConfigFieldTypeObject, Description: "Optional ordered classifier model fallback settings."},
				{Name: "routing", Type: infrastructure.ConfigFieldTypeObject, Description: "Routing policy weights and limits."},
				{Name: "models", Type: infrastructure.ConfigFieldTypeObject, Description: "Candidate provider/model matrix with capabilities and quality metadata."},
				{Name: "state_path", Type: infrastructure.ConfigFieldTypeString, Description: "Optional local state file path for future benchmark persistence."},
			},
		},
		Capabilities: capabilities,
	}
}

func callHost[T any](method string, payload any) (T, error) {
	var zero T
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return zero, err
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	var response C.cliproxy_buffer
	var requestPtr *C.uint8_t
	if len(rawPayload) > 0 {
		cPayload := C.CBytes(rawPayload)
		if cPayload == nil {
			return zero, fmt.Errorf("allocate host callback payload")
		}
		defer C.free(cPayload)
		requestPtr = (*C.uint8_t)(cPayload)
	}
	code := C.call_host_api(cMethod, requestPtr, C.size_t(len(rawPayload)), &response)
	var rawResponse []byte
	if response.ptr != nil && response.len > 0 {
		rawResponse = C.GoBytes(response.ptr, C.int(response.len))
	}
	if response.ptr != nil {
		C.free_host_buffer(response.ptr, response.len)
	}
	if code != 0 {
		return zero, fmt.Errorf("host callback %s returned code %d", method, int(code))
	}
	var env infrastructure.Envelope
	if err := json.Unmarshal(rawResponse, &env); err != nil {
		return zero, err
	}
	if !env.OK {
		if env.Error != nil {
			return zero, fmt.Errorf("%s: %s", env.Error.Code, env.Error.Message)
		}
		return zero, fmt.Errorf("host callback failed")
	}
	var out T
	if len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, &out); err != nil {
			return zero, err
		}
	}
	return out, nil
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}
