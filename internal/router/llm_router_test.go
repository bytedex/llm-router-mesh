package router

import (
	"context"
	"testing"

	"github.com/arnabdutta/llm-router-mesh/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "local-cheap", BaseURL: "http://localhost:11434/v1", Model: "qwen2.5:7b", Tier: "cheap"},
			{Name: "cloud-mid", BaseURL: "https://mid.example.com/v1", APIKey: "mid-key", Model: "mid-model", Tier: "mid"},
			{Name: "cloud-frontier", BaseURL: "https://frontier.example.com/v1", APIKey: "frontier-key", Model: "gpt-4o", Tier: "frontier"},
		},
		Routing: config.RoutingConfig{
			DefaultTier: "cheap",
			ComplexityKeywords: map[string][]string{
				"frontier": {"architect", "debug", "security"},
				"mid":      {"summarize", "explain"},
				"cheap":    {"format", "typo"},
			},
		},
	}
}

func TestLLMRouter_FrontierKeyword(t *testing.T) {
	r := NewLLMRouter(testConfig())
	decision, err := r.Route(context.Background(), "Help me debug this segfault")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Provider.Name != "cloud-frontier" {
		t.Errorf("got provider %s, want cloud-frontier", decision.Provider.Name)
	}
	if decision.Provider.Model != "gpt-4o" {
		t.Errorf("got model %s, want gpt-4o", decision.Provider.Model)
	}
}

func TestLLMRouter_CheapKeyword(t *testing.T) {
	r := NewLLMRouter(testConfig())
	decision, err := r.Route(context.Background(), "format this JSON for me")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Provider.Name != "local-cheap" {
		t.Errorf("got provider %s, want local-cheap", decision.Provider.Name)
	}
}

func TestLLMRouter_MidKeyword(t *testing.T) {
	r := NewLLMRouter(testConfig())
	decision, err := r.Route(context.Background(), "Summarize this document")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Provider.Name != "cloud-mid" {
		t.Errorf("got provider %s, want cloud-mid", decision.Provider.Name)
	}
}

func TestLLMRouter_DefaultTier(t *testing.T) {
	r := NewLLMRouter(testConfig())
	decision, err := r.Route(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Provider.Name != "local-cheap" {
		t.Errorf("got provider %s, want local-cheap (default)", decision.Provider.Name)
	}
}

func TestLLMRouter_FrontierBeatsOtherTiers(t *testing.T) {
	r := NewLLMRouter(testConfig())
	decision, err := r.Route(context.Background(), "format and debug this code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Provider.Name != "cloud-frontier" {
		t.Errorf("got provider %s, want cloud-frontier (frontier should win)", decision.Provider.Name)
	}
}

func TestLLMRouter_CaseInsensitive(t *testing.T) {
	r := NewLLMRouter(testConfig())
	decision, err := r.Route(context.Background(), "EXPLAIN this function")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Provider.Name != "cloud-mid" {
		t.Errorf("got provider %s, want cloud-mid", decision.Provider.Name)
	}
}
