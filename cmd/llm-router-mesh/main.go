package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arnabdutta/llm-router-mesh/pkg/cache"
	"github.com/arnabdutta/llm-router-mesh/pkg/config"
	"github.com/arnabdutta/llm-router-mesh/pkg/proxy"
	"github.com/arnabdutta/llm-router-mesh/pkg/router"
	"github.com/arnabdutta/llm-router-mesh/pkg/telemetry"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if err := telemetry.Setup(cfg.OTelEndpoint); err != nil {
		log.Fatalf("failed to initialize telemetry: %v", err)
	}
	defer telemetry.Shutdown()

	redisClient, err := cache.NewClient(cfg.RedisURL)
	if err != nil {
		log.Fatalf("failed to create redis client: %v", err)
	}
	defer redisClient.Close()

	var llmCache proxy.Cache
	if cfg.Cache.Enabled {
		ttl := time.Duration(cfg.Cache.TTLSecs) * time.Second
		rc := cache.NewRedisCache(redisClient, ttl)
		rc.EnsureIndex(context.Background())
		llmCache = rc
	} else {
		llmCache = cache.NoopCache{}
	}

	limits := make(map[string]cache.RateLimit)
	for _, p := range cfg.Providers {
		if p.RateLimit.RequestsPerMinute > 0 {
			limits[p.Name] = cache.RateLimit{
				MaxTokens:  p.RateLimit.RequestsPerMinute,
				RefillRate: float64(p.RateLimit.RequestsPerMinute) / 60.0,
			}
		}
	}
	rateLimiter := cache.NewRateLimiter(redisClient, limits)

	llmRouter := router.NewLLMRouter(cfg)
	llmProxy := proxy.NewLLMProxy(llmCache, llmRouter, rateLimiter, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/v1/chat/completions", llmProxy)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("received %s, shutting down...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	log.Printf("LLM Router Mesh listening on :%d", cfg.Port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
	log.Println("server stopped")
}
