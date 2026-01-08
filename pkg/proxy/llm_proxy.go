package proxy

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/arnabdutta/llm-router-mesh/pkg/config"
	"github.com/arnabdutta/llm-router-mesh/pkg/domain"
	"github.com/sony/gobreaker"
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
	Allow(ctx context.Context, provider string) (bool, error)
}

type LLMProxy struct {
	cache       Cache
	router      Router
	rateLimiter RateLimiter
	client      *http.Client
	cbs         map[string]*gobreaker.CircuitBreaker
	embedKey    string
	similarity  float32
}

func NewLLMProxy(cache Cache, router Router, rateLimiter RateLimiter, cfg *config.Config) *LLMProxy {
	cbs := make(map[string]*gobreaker.CircuitBreaker)
	for _, p := range cfg.Providers {
		cbs[p.Name] = gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:        p.Name,
			MaxRequests: 5,
			Interval:    10 * time.Second,
			Timeout:     30 * time.Second,
		})
	}
	return &LLMProxy{
		cache:       cache,
		router:      router,
		rateLimiter: rateLimiter,
		client:      &http.Client{Timeout: 60 * time.Second},
		cbs:         cbs,
		embedKey:    cfg.Cache.EmbeddingAPIKey,
		similarity:  cfg.Cache.SimilarityScore,
	}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (p *LLMProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request", http.StatusBadRequest)
		return
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	var msgs []message
	if err := json.Unmarshal(fields["messages"], &msgs); err != nil {
		http.Error(w, "invalid messages", http.StatusBadRequest)
		return
	}
	prompt := extractPrompt(msgs)

	isStream := false
	if streamJSON, ok := fields["stream"]; ok {
		json.Unmarshal(streamJSON, &isStream)
	}

	// 1. Semantic Cache check (Skip for streams for simplicity, or we can cache them later)
	var embedding []byte
	if !isStream && p.embedKey != "" {
		embedding, err = p.getEmbedding(ctx, prompt)
		if err == nil && embedding != nil {
			maxDist := float32(1.0) - p.similarity // e.g. 0.95 sim -> 0.05 dist
			if cached, _ := p.cache.GetSemantic(ctx, embedding, maxDist); cached != nil {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Cache", "HIT")
				w.Write(cached)
				return
			}
		}
	}

	// 2. Route
	decision, err := p.router.Route(ctx, prompt)
	if err != nil {
		http.Error(w, "routing failed", http.StatusInternalServerError)
		return
	}

	// 3. Fallbacks & Circuit Breaking
	var lastErr error
	for _, provider := range decision.Providers {
		// Rate limit
		if allowed, err := p.rateLimiter.Allow(ctx, provider.Name); err == nil && !allowed {
			lastErr = fmt.Errorf("rate limited on %s", provider.Name)
			continue
		}

		cb := p.cbs[provider.Name]
		_, err := cb.Execute(func() (interface{}, error) {
			return nil, p.forward(ctx, w, body, &provider, isStream, embedding)
		})

		if err == nil {
			return // Success!
		}
		lastErr = err
	}

	http.Error(w, fmt.Sprintf("all providers failed: %v", lastErr), http.StatusBadGateway)
}

func (p *LLMProxy) forward(ctx context.Context, w http.ResponseWriter, body []byte, provider *domain.Provider, isStream bool, embedding []byte) error {
	var fields map[string]json.RawMessage
	json.Unmarshal(body, &fields)
	modelJSON, _ := json.Marshal(provider.Model)
	fields["model"] = modelJSON
	patchedBody, _ := json.Marshal(fields)

	url := provider.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(patchedBody))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("upstream returned %d", resp.StatusCode)
	}

	w.Header().Set("X-Routed-To", provider.Name)
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)

	if isStream {
		// Streaming Support with Flusher
		flusher, ok := w.(http.Flusher)
		if !ok {
			io.Copy(w, resp.Body)
			return nil
		}

		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				flusher.Flush()
			}
			if err != nil {
				break
			}
		}
	} else {
		respBody, _ := io.ReadAll(resp.Body)
		w.Write(respBody)
		if resp.StatusCode == http.StatusOK && embedding != nil {
			cacheKey := fmt.Sprintf("llm:cache:%d", time.Now().UnixNano())
			p.cache.SetSemantic(context.Background(), cacheKey, embedding, respBody)
		}
	}
	return nil
}

func (p *LLMProxy) getEmbedding(ctx context.Context, text string) ([]byte, error) {
	reqBody := map[string]interface{}{
		"input": text,
		"model": "text-embedding-3-small",
	}
	reqBytes, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(reqBytes))
	req.Header.Set("Authorization", "Bearer "+p.embedKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Data) == 0 {
		return nil, fmt.Errorf("failed to decode embedding")
	}

	buf := new(bytes.Buffer)
	for _, f := range result.Data[0].Embedding {
		binary.Write(buf, binary.LittleEndian, f)
	}
	return buf.Bytes(), nil
}

func extractPrompt(msgs []message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}
