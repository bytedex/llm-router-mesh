package router

import (
	"context"
	"testing"

	"github.com/arnabdutta/llm-router-mesh/pkg/config"
)

func TestLLMRouter_Basic(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "local-cheap", BaseURL: "http://localhost:11434/v1", Model: "qwen2.5:7b", Tier: "cheap"},
		},
		Routing: config.RoutingConfig{
			DefaultTier: "cheap",
		},
	}
	r := NewLLMRouter(cfg)
	decision, err := r.Route(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Providers[0].Name != "local-cheap" {
		t.Errorf("got provider %s", decision.Providers[0].Name)
	}
}
