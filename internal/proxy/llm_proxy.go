package proxy

import (
	"context"
	"net/http"

	"github.com/arnabdutta/llm-router-mesh/internal/domain"
)

type Cache interface {
	EnsureIndex(ctx context.Context) error
	GetSemantic(ctx context.Context, embedding []byte, maxDistance float32) ([]byte, error)
	SetSemantic(ctx context.Context, key string, embedding []byte, response []byte) error
}

type Router interface {
	Route(ctx context.Context, prompt string) (*domain.RoutingDecision, error)
}

type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

type LLMProxy struct {}

func (p *LLMProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
}
