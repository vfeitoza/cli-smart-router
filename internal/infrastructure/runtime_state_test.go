package infrastructure

import (
	"testing"
	"time"
)

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
