package router

import (
	"context"

	"github.com/arnabdutta/llm-router-mesh/internal/domain"
)

type LLMRouter struct {}

func NewLLMRouter() *LLMRouter {
	return &LLMRouter{}
}

func (r *LLMRouter) Route(ctx context.Context, prompt string) (*domain.RoutingDecision, error) {
    return nil, nil
}
