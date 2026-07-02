package infrastructure

import (
	"encoding/json"

	"github.com/vfeitoza/cli-smart-router/internal/domain"
	"gopkg.in/yaml.v3"
)

// LifecycleRequest is the plugin.register/plugin.reconfigure payload.
type LifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}

// ParseConfig decodes plugin-owned YAML configuration.
func ParseConfig(raw []byte) (domain.Config, error) {
	var req LifecycleRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return domain.Config{}, err
		}
	}
	cfg := domain.DefaultConfig()
	if len(req.ConfigYAML) > 0 {
		if err := yaml.Unmarshal(req.ConfigYAML, &cfg); err != nil {
			return domain.Config{}, err
		}
	}
	return cfg.Normalize(), nil
}
