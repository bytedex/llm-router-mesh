package router

import (
	"context"
	"fmt"
	"strings"

	"github.com/arnabdutta/llm-router-mesh/internal/config"
	"github.com/arnabdutta/llm-router-mesh/internal/domain"
)

// LLMRouter inspects LLM prompts and decides which model should handle it based on complexity
type LLMRouter struct {
	providers   map[string][]domain.Provider
	keywords    map[string][]string
	defaultTier string
}

// NewLLMRouter creates a new LLMRouter from the loaded configuration
func NewLLMRouter(cfg *config.Config) *LLMRouter {
	providers := make(map[string][]domain.Provider)
	for _, p := range cfg.Providers {
		providers[p.Tier] = append(providers[p.Tier], domain.Provider{
			Name:    p.Name,
			BaseURL: p.BaseURL,
			APIKey:  p.APIKey,
			Model:   p.Model,
		})
	}
	return &LLMRouter{
		providers:   providers,
		keywords:    cfg.Routing.ComplexityKeywords,
		defaultTier: cfg.Routing.DefaultTier,
	}
}

// Route determines the optimal downstream LLM target based on keyword complexity matching.
// Tiers are checked in priority order (frontier -> mid -> cheap) so the most capable tier
// wins when a prompt matches keywords from multiple tiers.
func (r *LLMRouter) Route(ctx context.Context, prompt string) (*domain.RoutingDecision, error) {
	if prompt == "" {
		return &domain.RoutingDecision{
			Tier:      "basic",
			Providers: r.providers["basic"],
		}, nil
	}
	
	lower := strings.ToLower(prompt)

	for _, tier := range []string{"frontier", "mid", "cheap"} {
		keywords, ok := r.keywords[tier]
		if !ok {
			continue
		}
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				tierProviders, ok := r.providers[tier]
				if !ok || len(tierProviders) == 0 {
					continue
				}
				return &domain.RoutingDecision{
					Providers: tierProviders,
					Reason:    fmt.Sprintf("keyword %q matched tier %s", kw, tier),
				}, nil
			}
		}
	}

	tierProviders, ok := r.providers[r.defaultTier]
	if !ok || len(tierProviders) == 0 {
		return nil, fmt.Errorf("default tier %q has no provider", r.defaultTier)
	}
	return &domain.RoutingDecision{
		Providers: tierProviders,
		Reason:   "no keywords matched, using default tier",
	}, nil
}
