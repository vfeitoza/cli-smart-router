package infrastructure

import (
	"sync/atomic"

	"github.com/vfeitoza/cli-smart-router/internal/domain"
)

// ConfigStore stores the current normalized plugin configuration safely.
type ConfigStore struct {
	value atomic.Value
}

// NewConfigStore creates a config store with defaults.
func NewConfigStore() *ConfigStore {
	store := &ConfigStore{}
	store.Store(domain.DefaultConfig())
	return store
}

// Store replaces the current config.
func (s *ConfigStore) Store(cfg domain.Config) {
	if s == nil {
		return
	}
	s.value.Store(cfg.Normalize())
}

// Load returns the current config or defaults.
func (s *ConfigStore) Load() domain.Config {
	if s == nil {
		return domain.DefaultConfig()
	}
	if cfg, ok := s.value.Load().(domain.Config); ok {
		return cfg.Normalize()
	}
	return domain.DefaultConfig()
}
