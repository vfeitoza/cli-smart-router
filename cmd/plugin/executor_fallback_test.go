package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vfeitoza/cli-smart-router/internal/domain"
	"github.com/vfeitoza/cli-smart-router/internal/infrastructure"
)

func setupTestRouter(cfg domain.Config) func() {
	origConfig := configStore.Load()
	origExecutor := hostModelExecute
	origRuntime := runtimeState

	configStore.Store(cfg)
	runtimeState = infrastructure.NewRuntimeState()

	return func() {
		configStore.Store(origConfig)
		hostModelExecute = origExecutor
		runtimeState = origRuntime
	}
}

// Test 1: primary quota then secondary success
func TestExecutorFallbackPrimaryQuotaThenSecondarySuccess(t *testing.T) {
	cfg := domain.Config{
		Enabled: true,
		ExecutorFallback: domain.ExecutorFallbackConfig{
			Enabled:     true,
			MaxAttempts: 3,
		},
		Models: []domain.CandidateConfig{
			{Provider: "p1", Model: "m1"},
			{Provider: "p2", Model: "m2"},
		},
	}
	teardown := setupTestRouter(cfg)
	defer teardown()

	var attempts []string
	hostModelExecute = func(req infrastructure.HostModelExecutionRPCRequest) (infrastructure.HostModelExecutionResponse, error) {
		attempts = append(attempts, req.Model)
		if req.Model == "m1" {
			headers := http.Header{}
			headers.Set("Retry-After", "30")
			return infrastructure.HostModelExecutionResponse{
				StatusCode: http.StatusTooManyRequests,
				Headers:    headers,
				Body:       []byte(`{"error":"rate_limit_exceeded"}`),
			}, nil
		}
		return infrastructure.HostModelExecutionResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"result":"m2-success"}`),
		}, nil
	}

	execReq := infrastructure.ExecutorRPCRequest{
		ExecutorRequest: infrastructure.ExecutorRequest{
			SourceFormat: "openai",
			Payload:      []byte(`{"messages":[]}`),
		},
	}
	rawReq, _ := json.Marshal(execReq)
	rawResp, err := executeWithFallback(rawReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var env infrastructure.Envelope
	if err := json.Unmarshal(rawResp, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok response, got error: %+v", env.Error)
	}

	var execResp infrastructure.ExecutorResponse
	if err := json.Unmarshal(env.Result, &execResp); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if string(execResp.Payload) != `{"result":"m2-success"}` {
		t.Fatalf("unexpected payload: %s", string(execResp.Payload))
	}

	if len(attempts) != 2 || attempts[0] != "m1" || attempts[1] != "m2" {
		t.Fatalf("expected attempts [m1, m2], got %v", attempts)
	}

	// Verify primary m1 is now cooling down
	if !runtimeState.IsCooling("p1", "m1", time.Now()) {
		t.Fatal("expected primary m1 to be cooling down after quota error")
	}
}

// Test 2: next call avoids cooling primary
func TestExecutorFallbackNextCallAvoidsCoolingPrimary(t *testing.T) {
	cfg := domain.Config{
		Enabled: true,
		ExecutorFallback: domain.ExecutorFallbackConfig{
			Enabled:     true,
			MaxAttempts: 3,
		},
		Models: []domain.CandidateConfig{
			{Provider: "p1", Model: "m1"},
			{Provider: "p2", Model: "m2"},
		},
	}
	teardown := setupTestRouter(cfg)
	defer teardown()

	// Put m1 in cooldown
	runtimeState.SetCooldown("p1", "m1", time.Now().Add(60*time.Second))

	var attempts []string
	hostModelExecute = func(req infrastructure.HostModelExecutionRPCRequest) (infrastructure.HostModelExecutionResponse, error) {
		attempts = append(attempts, req.Model)
		return infrastructure.HostModelExecutionResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"result":"m2-success"}`),
		}, nil
	}

	execReq := infrastructure.ExecutorRPCRequest{
		ExecutorRequest: infrastructure.ExecutorRequest{
			SourceFormat: "openai",
			Payload:      []byte(`{"messages":[]}`),
		},
	}
	rawReq, _ := json.Marshal(execReq)
	rawResp, err := executeWithFallback(rawReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var env infrastructure.Envelope
	if err := json.Unmarshal(rawResp, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok response, got error: %+v", env.Error)
	}

	if len(attempts) != 1 || attempts[0] != "m2" {
		t.Fatalf("expected next call to avoid cooling primary m1 and call only [m2], got %v", attempts)
	}
}

// Test 3: healthy beyond max_attempts position remains reachable
func TestExecutorFallbackHealthyBeyondMaxAttemptsPositionRemainsReachable(t *testing.T) {
	cfg := domain.Config{
		Enabled: true,
		ExecutorFallback: domain.ExecutorFallbackConfig{
			Enabled:     true,
			MaxAttempts: 2, // Only 2 attempts allowed
		},
		Models: []domain.CandidateConfig{
			{Provider: "p1", Model: "m1"}, // cooling
			{Provider: "p2", Model: "m2"}, // fails
			{Provider: "p3", Model: "m3"}, // healthy beyond max_attempts in initial order
		},
	}
	teardown := setupTestRouter(cfg)
	defer teardown()

	// Mark m1 as cooling down
	runtimeState.SetCooldown("p1", "m1", time.Now().Add(60*time.Second))

	var attempts []string
	hostModelExecute = func(req infrastructure.HostModelExecutionRPCRequest) (infrastructure.HostModelExecutionResponse, error) {
		attempts = append(attempts, req.Model)
		if req.Model == "m2" {
			return infrastructure.HostModelExecutionResponse{
				StatusCode: http.StatusServiceUnavailable,
			}, nil
		}
		return infrastructure.HostModelExecutionResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"result":"m3-success"}`),
		}, nil
	}

	execReq := infrastructure.ExecutorRPCRequest{
		ExecutorRequest: infrastructure.ExecutorRequest{
			SourceFormat: "openai",
			Payload:      []byte(`{"messages":[]}`),
		},
	}
	rawReq, _ := json.Marshal(execReq)
	rawResp, err := executeWithFallback(rawReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var env infrastructure.Envelope
	if err := json.Unmarshal(rawResp, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok response, got error: %+v", env.Error)
	}

	// Because m1 is cooling, healthy m2 and m3 are tried before cooling m1.
	// Capped to max_attempts = 2: candidates are [m2, m3].
	// When m2 fails, m3 is reachable and succeeds!
	if len(attempts) != 2 || attempts[0] != "m2" || attempts[1] != "m3" {
		t.Fatalf("expected attempts [m2, m3], got %v", attempts)
	}
}

// Test 4: RPC cooldown string
func TestExecutorFallbackRPCCooldownString(t *testing.T) {
	cfg := domain.Config{
		Enabled: true,
		ExecutorFallback: domain.ExecutorFallbackConfig{
			Enabled:     true,
			MaxAttempts: 3,
		},
		Models: []domain.CandidateConfig{
			{Provider: "p1", Model: "m1"},
			{Provider: "p2", Model: "m2"},
		},
	}
	teardown := setupTestRouter(cfg)
	defer teardown()

	var attempts []string
	hostModelExecute = func(req infrastructure.HostModelExecutionRPCRequest) (infrastructure.HostModelExecutionResponse, error) {
		attempts = append(attempts, req.Model)
		if req.Model == "m1" {
			return infrastructure.HostModelExecutionResponse{}, errors.New("model_cooldown: m1 is cooling down")
		}
		return infrastructure.HostModelExecutionResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"result":"m2-success"}`),
		}, nil
	}

	execReq := infrastructure.ExecutorRPCRequest{
		ExecutorRequest: infrastructure.ExecutorRequest{
			SourceFormat: "openai",
			Payload:      []byte(`{"messages":[]}`),
		},
	}
	rawReq, _ := json.Marshal(execReq)
	rawResp, err := executeWithFallback(rawReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var env infrastructure.Envelope
	if err := json.Unmarshal(rawResp, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok response, got error: %+v", env.Error)
	}

	if len(attempts) != 2 || attempts[0] != "m1" || attempts[1] != "m2" {
		t.Fatalf("expected [m1, m2], got %v", attempts)
	}

	// Verify m1 was placed in cooldown
	if !runtimeState.IsCooling("p1", "m1", time.Now()) {
		t.Fatal("expected m1 to be in cooldown after model_cooldown RPC error")
	}
}

// Test 5: nonretryable RPC error does not return empty success
func TestExecutorFallbackNonretryableRPCErrorDoesNotReturnEmptySuccess(t *testing.T) {
	cfg := domain.Config{
		Enabled: true,
		ExecutorFallback: domain.ExecutorFallbackConfig{
			Enabled:     true,
			MaxAttempts: 3,
		},
		Models: []domain.CandidateConfig{
			{Provider: "p1", Model: "m1"},
			{Provider: "p2", Model: "m2"},
		},
	}
	teardown := setupTestRouter(cfg)
	defer teardown()

	var attempts []string
	hostModelExecute = func(req infrastructure.HostModelExecutionRPCRequest) (infrastructure.HostModelExecutionResponse, error) {
		attempts = append(attempts, req.Model)
		return infrastructure.HostModelExecutionResponse{}, errors.New("invalid_argument: unknown model format")
	}

	execReq := infrastructure.ExecutorRPCRequest{
		ExecutorRequest: infrastructure.ExecutorRequest{
			SourceFormat: "openai",
			Payload:      []byte(`{"messages":[]}`),
		},
	}
	rawReq, _ := json.Marshal(execReq)
	rawResp, err := executeWithFallback(rawReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var env infrastructure.Envelope
	if err := json.Unmarshal(rawResp, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.OK {
		t.Fatalf("expected error envelope for nonretryable RPC error, got OK response: %s", string(rawResp))
	}
	if env.Error == nil || env.Error.Code != "host_execution_failed" {
		t.Fatalf("expected host_execution_failed error, got: %+v", env.Error)
	}

	// Nonretryable error must not advance to secondary candidate
	if len(attempts) != 1 || attempts[0] != "m1" {
		t.Fatalf("expected nonretryable error to fail immediately on m1, got attempts: %v", attempts)
	}
}

// Test 6: recovered cooldown eviction
func TestExecutorFallbackRecoveredCooldownEviction(t *testing.T) {
	cfg := domain.Config{
		Enabled: true,
		ExecutorFallback: domain.ExecutorFallbackConfig{
			Enabled:     true,
			MaxAttempts: 3,
		},
		Models: []domain.CandidateConfig{
			{Provider: "p1", Model: "m1"},
			{Provider: "p2", Model: "m2"},
		},
	}
	teardown := setupTestRouter(cfg)
	defer teardown()

	// Set m1 cooldown that already expired
	past := time.Now().Add(-5 * time.Second)
	runtimeState.SetCooldown("p1", "m1", past)

	// Inquire at now: should report not cooling and evict entry
	if runtimeState.IsCooling("p1", "m1", time.Now()) {
		t.Fatal("expected expired cooldown to be recovered/not cooling")
	}

	var attempts []string
	hostModelExecute = func(req infrastructure.HostModelExecutionRPCRequest) (infrastructure.HostModelExecutionResponse, error) {
		attempts = append(attempts, req.Model)
		return infrastructure.HostModelExecutionResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"result":"m1-success"}`),
		}, nil
	}

	execReq := infrastructure.ExecutorRPCRequest{
		ExecutorRequest: infrastructure.ExecutorRequest{
			SourceFormat: "openai",
			Payload:      []byte(`{"messages":[]}`),
		},
	}
	rawReq, _ := json.Marshal(execReq)
	_, err := executeWithFallback(rawReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Because m1 recovered, it was attempted first as primary
	if len(attempts) != 1 || attempts[0] != "m1" {
		t.Fatalf("expected recovered m1 to be attempted first, got %v", attempts)
	}
}

// Test 7: cap cached policy chains
func TestExecutorFallbackCapCachedPolicyChains(t *testing.T) {
	cfg := domain.Config{
		Enabled: true,
		ExecutorFallback: domain.ExecutorFallbackConfig{
			Enabled:     true,
			MaxAttempts: 2, // Cap at 2 attempts
		},
		Models: []domain.CandidateConfig{
			{Provider: "p1", Model: "m1"},
			{Provider: "p2", Model: "m2"},
			{Provider: "p3", Model: "m3"},
			{Provider: "p4", Model: "m4"},
			{Provider: "p5", Model: "m5"},
		},
	}
	teardown := setupTestRouter(cfg)
	defer teardown()

	// Store a 5-model policy chain in cache for this session
	runtimeState.SetFallbackChain("session-xyz",
		[]string{"p1", "p2", "p3", "p4", "p5"},
		[]string{"m1", "m2", "m3", "m4", "m5"},
	)

	req := infrastructure.ExecutorRPCRequest{
		ExecutorRequest: infrastructure.ExecutorRequest{
			Metadata: map[string]any{
				"execution_session_id": "session-xyz",
			},
		},
	}

	providers, models := executorCandidateChain(cfg, req)
	if len(models) != 2 || len(providers) != 2 {
		t.Fatalf("expected cached policy chain to be capped to max_attempts (2), got %d models: %v", len(models), models)
	}
	if models[0] != "m1" || models[1] != "m2" {
		t.Fatalf("expected [m1, m2], got %v", models)
	}
}

func TestExecutorFallbackMissingPolicyChainFailsClosed(t *testing.T) {
	cfg := domain.Config{
		Enabled:          true,
		ExecutorFallback: domain.ExecutorFallbackConfig{Enabled: true, MaxAttempts: 2},
		Models:           []domain.CandidateConfig{{Provider: "disallowed", Model: "outside-policy"}},
	}
	teardown := setupTestRouter(cfg)
	defer teardown()
	hostModelExecute = func(req infrastructure.HostModelExecutionRPCRequest) (infrastructure.HostModelExecutionResponse, error) {
		t.Fatal("missing policy chain must not execute configured models")
		return infrastructure.HostModelExecutionResponse{}, nil
	}
	req := infrastructure.ExecutorRPCRequest{
		ExecutorRequest: infrastructure.ExecutorRequest{
			Metadata: map[string]any{"execution_session_id": "evicted-session"},
		},
	}
	raw, _ := json.Marshal(req)
	response, err := executeWithFallback(raw)
	if err != nil || !strings.Contains(string(response), "executor_fallback_failed") {
		t.Fatalf("expected explicit fallback error, got %s, %v", response, err)
	}
}

func TestExecutorFallbackPreservesRequestBody(t *testing.T) {
	cfg := domain.Config{
		Enabled: true,
		ExecutorFallback: domain.ExecutorFallbackConfig{
			Enabled:     true,
			MaxAttempts: 2,
		},
		Models: []domain.CandidateConfig{
			{Provider: "p1", Model: "m1"},
		},
	}
	teardown := setupTestRouter(cfg)
	defer teardown()

	expectedBody := []byte(`{"model":"custom","messages":[{"role":"user","content":"hello world"}]}`)
	var receivedBody []byte
	hostModelExecute = func(req infrastructure.HostModelExecutionRPCRequest) (infrastructure.HostModelExecutionResponse, error) {
		receivedBody = req.Body
		return infrastructure.HostModelExecutionResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"result":"ok"}`),
		}, nil
	}

	execReq := infrastructure.ExecutorRPCRequest{
		ExecutorRequest: infrastructure.ExecutorRequest{
			SourceFormat:    "openai",
			OriginalRequest: expectedBody,
		},
	}
	rawReq, _ := json.Marshal(execReq)
	_, err := executeWithFallback(rawReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(receivedBody) != string(expectedBody) {
		t.Fatalf("expected preserved source body %s, got %s", string(expectedBody), string(receivedBody))
	}
}
