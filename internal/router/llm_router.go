package router

import (
	"context"

	"github.com/arnabdutta/llm-router-mesh/internal/domain"
	"github.com/arnabdutta/llm-router-mesh/internal/config"
)

type LLMRouter struct {
	providers   map[string][]domain.Provider
	keywords    map[string][]string
	defaultTier string
}

func NewLLMRouter(cfg *config.Config) *LLMRouter {
	return &LLMRouter{
		providers:   make(map[string][]domain.Provider),
		keywords:    make(map[string][]string),
		defaultTier: "basic",
	}
}

func (r *LLMRouter) Route(ctx context.Context, prompt string) (*domain.RoutingDecision, error) {
    return nil, nil
}
