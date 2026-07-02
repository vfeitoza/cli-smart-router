package domain

import (
	"strings"
)

const (
	// DefaultVirtualModel is used when the plugin configuration omits virtual_model.
	DefaultVirtualModel = "router:auto"
	// DefaultStrategy is the safest V1 strategy because it never calls a classifier.
	DefaultStrategy = "capability"
	// DefaultPreference balances cost and quality when preference is omitted.
	DefaultPreference = "balanced"

	// PreferenceCost biases routing toward the cheapest acceptable model.
	PreferenceCost = "cost"
	// PreferenceBalanced balances cost and quality.
	PreferenceBalanced = "balanced"
	// PreferenceQuality biases routing toward the highest quality model.
	PreferenceQuality = "quality"
)

// Config contains plugin-owned configuration parsed from config_yaml.
type Config struct {
	Enabled          bool                   `yaml:"enabled"`
	VirtualModel     string                 `yaml:"virtual_model"`
	Strategy         string                 `yaml:"strategy"`
	Preference       string                 `yaml:"preference"`
	StatePath        string                 `yaml:"state_path"`
	Debug            DebugConfig            `yaml:"debug"`
	Catalog          CatalogConfig          `yaml:"catalog"`
	Pricing          PricingConfig          `yaml:"pricing"`
	Cache            CacheConfig            `yaml:"cache"`
	ExecutorFallback ExecutorFallbackConfig `yaml:"executor_fallback"`
	Classifier       ClassifierConfig       `yaml:"classifier"`
	Routing          RoutingConfig          `yaml:"routing"`
	Models           []CandidateConfig      `yaml:"models"`
}

// DebugConfig controls non-sensitive route decision logs.
type DebugConfig struct {
	Enabled bool   `yaml:"enabled"`
	LogPath string `yaml:"log_path"`
}

// CatalogConfig controls live catalog refresh from CLIProxyAPI /v1/models.
type CatalogConfig struct {
	Source             string `yaml:"source"`
	BaseURL            string `yaml:"base_url"`
	APIKey             string `yaml:"api_key"`
	RefreshInterval    string `yaml:"refresh_interval"`
	IncludeRouterModel bool   `yaml:"include_router_model"`
}

// PricingConfig controls external model pricing refresh.
type PricingConfig struct {
	Enabled         bool   `yaml:"enabled"`
	URL             string `yaml:"url"`
	RefreshInterval string `yaml:"refresh_interval"`
}

// CacheConfig controls route decision caching.
type CacheConfig struct {
	Enabled    bool   `yaml:"enabled"`
	MaxEntries int    `yaml:"max_entries"`
	TTL        string `yaml:"ttl"`
}

// ExecutorFallbackConfig controls same-request non-streaming fallback.
type ExecutorFallbackConfig struct {
	Enabled     bool `yaml:"enabled"`
	MaxAttempts int  `yaml:"max_attempts"`
}

// ClassifierConfig controls optional LLM-based classification.
type ClassifierConfig struct {
	Enabled     bool              `yaml:"enabled"`
	Models      []ClassifierModel `yaml:"models"`
	Timeout     string            `yaml:"timeout"`
	MaxAttempts int               `yaml:"max_attempts"`
}

// ClassifierModel is one ordered fallback classifier target.
type ClassifierModel struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// RoutingConfig controls policy-level routing preferences.
type RoutingConfig struct {
	PreferLowCost           bool    `yaml:"prefer_low_cost"`
	PreferLowLatency        bool    `yaml:"prefer_low_latency"`
	PreferHighQuality       bool    `yaml:"prefer_high_quality"`
	MaxCostPerRequest       float64 `yaml:"max_cost_per_request"`
	MaxInputTokens          int64   `yaml:"max_input_tokens"`
	KeepSameModelPerSession bool    `yaml:"keep_same_model_per_session"`
	AllowFallback           bool    `yaml:"allow_fallback"`
	SwitchThreshold         float64 `yaml:"switch_threshold"`
	BenchmarkWeight         float64 `yaml:"benchmark_weight"`
	LLMRouterWeight         float64 `yaml:"llm_router_weight"`
	CapabilityWeight        float64 `yaml:"capability_weight"`
}

// CandidateConfig describes one routable provider/model candidate.
type CandidateConfig struct {
	Provider     string   `yaml:"provider"`
	Model        string   `yaml:"model"`
	Capabilities []string `yaml:"capabilities"`
	Cost         string   `yaml:"cost"`
	Quality      string   `yaml:"quality"`
}

// DefaultConfig returns a safe default configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:      true,
		VirtualModel: DefaultVirtualModel,
		Strategy:     DefaultStrategy,
		Preference:   DefaultPreference,
		Routing: RoutingConfig{
			PreferLowCost:           true,
			KeepSameModelPerSession: true,
			AllowFallback:           true,
			SwitchThreshold:         0.15,
			BenchmarkWeight:         0.4,
			LLMRouterWeight:         0.3,
			CapabilityWeight:        0.3,
		},
		Cache:            CacheConfig{Enabled: true, MaxEntries: 1024},
		ExecutorFallback: ExecutorFallbackConfig{MaxAttempts: 3},
	}
}

// Normalize fills defaults and trims user-provided strings.
func (c Config) Normalize() Config {
	if strings.TrimSpace(c.VirtualModel) == "" {
		c.VirtualModel = DefaultVirtualModel
	}
	if strings.TrimSpace(c.Strategy) == "" {
		c.Strategy = DefaultStrategy
	}
	c.VirtualModel = strings.TrimSpace(c.VirtualModel)
	c.Strategy = strings.ToLower(strings.TrimSpace(c.Strategy))
	c.Preference = strings.ToLower(strings.TrimSpace(c.Preference))
	switch c.Preference {
	case PreferenceCost, PreferenceBalanced, PreferenceQuality:
	default:
		c.Preference = DefaultPreference
	}
	c.StatePath = strings.TrimSpace(c.StatePath)
	c.Debug.LogPath = strings.TrimSpace(c.Debug.LogPath)
	c.Catalog.Source = strings.TrimSpace(c.Catalog.Source)
	c.Catalog.BaseURL = strings.TrimRight(strings.TrimSpace(c.Catalog.BaseURL), "/")
	c.Catalog.APIKey = strings.TrimSpace(c.Catalog.APIKey)
	c.Catalog.RefreshInterval = strings.TrimSpace(c.Catalog.RefreshInterval)
	c.Pricing.URL = strings.TrimSpace(c.Pricing.URL)
	c.Pricing.RefreshInterval = strings.TrimSpace(c.Pricing.RefreshInterval)
	if c.Cache.MaxEntries <= 0 {
		c.Cache.MaxEntries = 1024
	}
	c.Cache.TTL = strings.TrimSpace(c.Cache.TTL)
	if c.ExecutorFallback.MaxAttempts <= 0 {
		c.ExecutorFallback.MaxAttempts = 3
	}
	c.Classifier.Timeout = strings.TrimSpace(c.Classifier.Timeout)
	for i := range c.Classifier.Models {
		c.Classifier.Models[i].Provider = strings.ToLower(strings.TrimSpace(c.Classifier.Models[i].Provider))
		c.Classifier.Models[i].Model = strings.TrimSpace(c.Classifier.Models[i].Model)
	}
	for i := range c.Models {
		c.Models[i].Provider = strings.ToLower(strings.TrimSpace(c.Models[i].Provider))
		c.Models[i].Model = strings.TrimSpace(c.Models[i].Model)
		c.Models[i].Cost = strings.ToLower(strings.TrimSpace(c.Models[i].Cost))
		c.Models[i].Quality = strings.ToLower(strings.TrimSpace(c.Models[i].Quality))
	}
	return c
}

// EnabledForRouting reports whether the plugin should handle route requests.
func (c Config) EnabledForRouting() bool {
	return c.Normalize().Enabled
}
