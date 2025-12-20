package config

type Config struct {
	Port         int    `yaml:"port"`
	RedisURL     string `yaml:"redis_url"`
	OTelEndpoint string `yaml:"otel_endpoint"`
}
