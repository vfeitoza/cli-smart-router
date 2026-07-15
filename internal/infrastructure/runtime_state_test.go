package infrastructure

import (
	"testing"
	"time"
)

func TestRuntimeStateLastDecisionSnapshot(t *testing.T) {
	state := NewRuntimeState()
	if snap := state.Snapshot(); snap.LastDecision != nil {
		t.Fatalf("expected no decision initially, got %#v", snap.LastDecision)
	}
	state.SetLastDecision(DecisionSnapshot{
		Task:           "coding",
		Language:       "go",
		Score:          42,
		Policy:         "rule#1 task=coding language=go",
		Provider:       "codex",
		Model:          "gpt-5.4-mini",
		Reason:         "matched",
		Matched:        true,
		DecisionTimeUS: 128,
	})
	snap := state.Snapshot()
	if snap.LastDecision == nil {
		t.Fatal("expected last decision to be set")
	}
	got := snap.LastDecision
	if got.Task != "coding" || got.Language != "go" || got.Score != 42 || got.Model != "gpt-5.4-mini" || !got.Matched || got.DecisionTimeUS != 128 {
		t.Fatalf("unexpected snapshot = %#v", got)
	}
}

func TestRuntimeStateCacheAndSession(t *testing.T) {
	state := NewRuntimeState()
	entry := RouteCacheEntry{Provider: "codex", Model: "gpt-5.4-mini", Reason: "test"}
	state.SetCachedRoute("key", entry, 10)
	got, ok := state.GetCachedRoute("key", 0)
	if !ok || got.Provider != "codex" || got.Model != "gpt-5.4-mini" {
		t.Fatalf("GetCachedRoute() = %#v, %v; want codex/gpt-5.4-mini", got, ok)
	}
	state.SetSessionRoute("session-1", entry)
	got, ok = state.GetSessionRoute("session-1")
	if !ok || got.Provider != "codex" || got.Model != "gpt-5.4-mini" {
		t.Fatalf("GetSessionRoute() = %#v, %v; want codex/gpt-5.4-mini", got, ok)
	}
}

func TestRuntimeStateCacheEvictsLeastRecentlyUsed(t *testing.T) {
	state := NewRuntimeState()
	state.SetCachedRoute("a", RouteCacheEntry{Model: "m-a"}, 2)
	state.SetCachedRoute("b", RouteCacheEntry{Model: "m-b"}, 2)
	// Touch "a" so "b" becomes the least recently used.
	if _, ok := state.GetCachedRoute("a", 0); !ok {
		t.Fatal("expected key a to be present")
	}
	state.SetCachedRoute("c", RouteCacheEntry{Model: "m-c"}, 2)
	if _, ok := state.GetCachedRoute("b", 0); ok {
		t.Fatal("expected least recently used key b to be evicted")
	}
	if _, ok := state.GetCachedRoute("a", 0); !ok {
		t.Fatal("expected recently used key a to survive")
	}
}

func TestRuntimeStateCacheExpiresByTTL(t *testing.T) {
	state := NewRuntimeState()
	state.SetCachedRoute("k", RouteCacheEntry{Model: "m", CreatedAt: time.Now().Add(-2 * time.Hour)}, 10)
	if _, ok := state.GetCachedRoute("k", time.Hour); ok {
		t.Fatal("expected expired entry to be dropped")
	}
}

func TestRuntimeStatePersistsNonSensitiveData(t *testing.T) {
	state := NewRuntimeState()
	state.SetCatalog([]string{"gpt-5.4-mini"}, "")
	state.SetPricing(123, "")
	state.RecordUsage(UsageRecord{Provider: "codex", Model: "gpt-5.4-mini", Detail: UsageDetail{TotalTokens: 7}})
	path := t.TempDir() + "/state.json"
	if err := state.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile() error = %v", err)
	}
	loaded := NewRuntimeState()
	if err := loaded.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}
	snapshot := loaded.Snapshot()
	if len(snapshot.Catalog.Models) != 1 || snapshot.Pricing.Bytes != 123 || snapshot.Usage["codex/gpt-5.4-mini"].TotalTokens != 7 {
		t.Fatalf("loaded snapshot = %#v", snapshot)
	}
}

func TestRuntimeStateFallbackChain(t *testing.T) {
	state := NewRuntimeState()
	if _, ok := state.GetFallbackChain("s1"); ok {
		t.Fatalf("expected no chain initially")
	}
	state.SetFallbackChain("s1", []string{"codex", "claude"}, []string{"gpt-5.4-mini", "claude-sonnet-5"})
	chain, ok := state.GetFallbackChain("s1")
	if !ok || len(chain.Models) != 2 {
		t.Fatalf("expected stored chain, got %+v ok=%v", chain, ok)
	}
	if chain.Providers[1] != "claude" || chain.Models[0] != "gpt-5.4-mini" {
		t.Fatalf("unexpected chain contents: %+v", chain)
	}
	if _, ok := state.GetFallbackChain(""); ok {
		t.Fatalf("expected empty session id to return no chain")
	}
}
