package main

import (
	"net/http"
	"testing"

	"github.com/vfeitoza/cli-smart-router/internal/infrastructure"
)

func TestRoutingTaskPrefersHeaderThenPromptTag(t *testing.T) {
	req := infrastructure.ModelRouteRequest{Headers: http.Header{"X-Router-Task": {"review"}}}
	if got := routingTask(req, "[router-task: coding] implemente conforme o plano"); got.String() != "review" {
		t.Fatalf("expected header review, got %q", got)
	}

	req.Headers.Set("X-Router-Task", "invalid")
	if got := routingTask(req, "[router-task: coding] implemente conforme o plano"); got.String() != "coding" {
		t.Fatalf("expected prompt tag coding, got %q", got)
	}

	req.Headers.Del("X-Router-Task")
	req.Headers.Set("X-Router-Agent", "implementer")
	if got := routingTask(req, "[router-task: review] revise o diff"); got.String() != "coding" {
		t.Fatalf("expected agent header coding, got %q", got)
	}

	req.Headers.Del("X-Router-Agent")
	if got := routingTask(req, "[router-agent: reviewer] implemente conforme o plano"); got.String() != "review" {
		t.Fatalf("expected agent tag review, got %q", got)
	}
}

func TestRouteCacheKeyIncludesTaskOverride(t *testing.T) {
	req := infrastructure.ModelRouteRequest{RequestedModel: "auto-model", Body: []byte(`{"messages":[{"role":"user","content":"implemente conforme o plano"}]}`), Headers: http.Header{}}
	withoutOverride := routeCacheKey(req)
	req.Headers.Set("X-Router-Task", "coding")
	if withOverride := routeCacheKey(req); withOverride == withoutOverride {
		t.Fatal("expected task override to change cache key")
	}
}
