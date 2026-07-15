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
	Routes           []RouteRule            `yaml:"routes"`
	Models           []CandidateConfig      `yaml:"models"`
}

// RouteRule is one declarative routing rule read from config `routes:`. When the
// `when` conditions all match the current request facts, the Policy Engine routes
// to `model` (and optionally `provider`). Rules are matched by specificity, so
// unset conditions are treated as wildcards. Absent `routes:` keeps legacy behavior.
type RouteRule struct {
	When  RouteCondition `yaml:"when"`
	Model string         `yaml:"model"`
	// Provider disambiguates when the same model id exists under multiple providers.
	Provider string `yaml:"provider"`
}

// RouteCondition is the `when` block of a RouteRule. Every field is optional; an
// empty field is a wildcard that matches anything. This keeps rules terse and
// avoids scattered conditionals: the Policy Engine evaluates them table-driven.
type RouteCondition struct {
	// Task matches the detected Intent (e.g. coding, review, security).
	Task string `yaml:"task"`
	// Language matches the detected programming language (e.g. go, python).
	Language string `yaml:"language"`
	// Complexity matches a coarse bucket: low, medium, high, very_high.
	Complexity string `yaml:"complexity"`
	// ComplexityMin/Max match the numeric complexity score range [0,100].
	ComplexityMin *int `yaml:"complexity_min"`
	ComplexityMax *int `yaml:"complexity_max"`
	// MinFiles matches when the request references at least this many files.
	MinFiles *int `yaml:"min_files"`
	// HasDiff, when set, matches only requests that do (true) or do not (false) carry a diff.
	HasDiff *bool `yaml:"has_diff"`
	// Stream, when set, matches only streaming (true) or non-streaming (false) requests.
	Stream *bool `yaml:"stream"`
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
	for i := range c.Routes {
		c.Routes[i].Model = strings.TrimSpace(c.Routes[i].Model)
		c.Routes[i].Provider = strings.ToLower(strings.TrimSpace(c.Routes[i].Provider))
		c.Routes[i].When.Task = strings.ToLower(strings.TrimSpace(c.Routes[i].When.Task))
		c.Routes[i].When.Language = strings.ToLower(strings.TrimSpace(c.Routes[i].When.Language))
		c.Routes[i].When.Complexity = strings.ToLower(strings.TrimSpace(c.Routes[i].When.Complexity))
	}
	return c
}

// EnabledForRouting reports whether the plugin should handle route requests.
func (c Config) EnabledForRouting() bool {
	return c.Normalize().Enabled
}

// Candidates converts the configured models into normalized routing candidates.
// It is the single conversion shared by the Router, Policy Engine wiring, and
// preference tiebreak so no caller re-implements the models -> candidates loop.
func (c Config) Candidates() []Candidate {
	out := make([]Candidate, 0, len(c.Models))
	for _, item := range c.Models {
		out = append(out, CandidateFromConfig(item))
	}
	return out
}
