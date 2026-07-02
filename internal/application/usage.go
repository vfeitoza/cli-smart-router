package application

import (
	"sync"

	"github.com/vfeitoza/cli-smart-router/internal/infrastructure"
)

// UsageStats stores minimal in-memory usage counters for V1 observability.
type UsageStats struct {
	Requests int64 `json:"requests"`
	Failures int64 `json:"failures"`
}

// UsageLearner records usage side-channel events.
type UsageLearner struct {
	mu    sync.Mutex
	stats UsageStats
}

// Record updates usage counters without storing request or response bodies.
func (u *UsageLearner) Record(record infrastructure.UsageRecord) {
	if u == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.stats.Requests++
	if record.Failed {
		u.stats.Failures++
	}
}

// Snapshot returns a copy of current usage counters.
func (u *UsageLearner) Snapshot() UsageStats {
	if u == nil {
		return UsageStats{}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.stats
}
