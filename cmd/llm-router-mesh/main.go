package main

import (
	"log"

	"github.com/arnabdutta/llm-router-mesh/internal/config"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	_ = cfg
}
