package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arnabdutta/llm-router-mesh/internal/domain"
)

type mockCache struct {
	store map[string][]byte
}

func (m *mockCache) Get(_ context.Context, key string) ([]byte, error) {
	v := m.store[key]
	return v, nil
}
func (m *mockCache) Set(_ context.Context, key string, value []byte) error {
	m.store[key] = value
	return nil
}

type mockRouter struct {
	decision *domain.RoutingDecision
}

func (m *mockRouter) Route(_ context.Context, _ string) (*domain.RoutingDecision, error) {
	return m.decision, nil
}

type mockRateLimiter struct {
	allowed bool
}

func (m *mockRateLimiter) Allow(_ context.Context, _ string) (bool, error) {
	return m.allowed, nil
}

func TestLLMProxy_CacheHit(t *testing.T) {
	msgs := []message{{Role: "user", Content: "cached question"}}
	key := computeCacheKey(msgs)

	mc := &mockCache{store: map[string][]byte{
		key: []byte(`{"id":"cached","choices":[{"message":{"role":"assistant","content":"cached answer"}}]}`),
	}}

	p := NewLLMProxy(mc, &mockRouter{}, &mockRateLimiter{allowed: true})

	body := `{"messages":[{"role":"user","content":"cached question"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if w.Header().Get("X-Cache") != "HIT" {
		t.Errorf("X-Cache: got %s, want HIT", w.Header().Get("X-Cache"))
	}
}

func TestLLMProxy_CacheMiss_ForwardsToProvider(t *testing.T) {
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "test-model" {
			t.Errorf("downstream model: got %v, want test-model", req["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"live","choices":[{"message":{"role":"assistant","content":"live answer"}}]}`))
	}))
	defer downstream.Close()

	mc := &mockCache{store: make(map[string][]byte)}
	mr := &mockRouter{decision: &domain.RoutingDecision{
		Provider: domain.Provider{Name: "test-provider", BaseURL: downstream.URL, Model: "test-model"},
		Reason:   "test",
	}}

	p := NewLLMProxy(mc, mr, &mockRateLimiter{allowed: true})

	body := `{"messages":[{"role":"user","content":"live question"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if w.Header().Get("X-Cache") != "MISS" {
		t.Errorf("X-Cache: got %s, want MISS", w.Header().Get("X-Cache"))
	}
	if w.Header().Get("X-Routed-To") != "test-provider" {
		t.Errorf("X-Routed-To: got %s, want test-provider", w.Header().Get("X-Routed-To"))
	}
	if len(mc.store) != 1 {
		t.Error("response should be cached after a miss")
	}
}

func TestLLMProxy_RateLimited(t *testing.T) {
	mc := &mockCache{store: make(map[string][]byte)}
	mr := &mockRouter{decision: &domain.RoutingDecision{
		Provider: domain.Provider{Name: "p"},
		Reason:   "test",
	}}

	p := NewLLMProxy(mc, mr, &mockRateLimiter{allowed: false})

	body := `{"messages":[{"role":"user","content":"rate limited"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status: got %d, want 429", w.Code)
	}
}

func TestLLMProxy_ProviderDown(t *testing.T) {
	mc := &mockCache{store: make(map[string][]byte)}
	mr := &mockRouter{decision: &domain.RoutingDecision{
		Provider: domain.Provider{Name: "dead", BaseURL: "http://127.0.0.1:1"},
		Reason:   "test",
	}}

	p := NewLLMProxy(mc, mr, &mockRateLimiter{allowed: true})

	body := `{"messages":[{"role":"user","content":"to dead provider"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status: got %d, want 502", w.Code)
	}
}

func TestLLMProxy_MethodNotAllowed(t *testing.T) {
	p := NewLLMProxy(&mockCache{store: make(map[string][]byte)}, &mockRouter{}, &mockRateLimiter{allowed: true})

	req := httptest.NewRequest("GET", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want 405", w.Code)
	}
}
