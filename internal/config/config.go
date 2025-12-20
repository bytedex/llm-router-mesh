package config

type Config struct {
	Port         int              `yaml:"port"`
	RedisURL     string           `yaml:"redis_url"`
	OTelEndpoint string           `yaml:"otel_endpoint"`
	Cache        CacheConfig      `yaml:"cache"`
	Providers    []ProviderConfig `yaml:"providers"`
}

type CacheConfig struct {
	Enabled         bool    `yaml:"enabled"`
	TTLSecs         int     `yaml:"ttl_seconds"`
}

type ProviderConfig struct {
	Name    string `yaml:"name"`
	Tier    string `yaml:"tier"`
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
}
