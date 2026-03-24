package main

import (
	"context"
	"log"
	"os"

	"github.com/caarlos0/env/v11"
	"github.com/prokhorind/google-classroom-mcp/classroom"
)

type Config struct {
	ClientID     string `env:"GOOGLE_CLIENT_ID,required"`
	ClientSecret string `env:"GOOGLE_CLIENT_SECRET,required"`
	TokenFile    string `env:"GOOGLE_TOKEN_FILE" envDefault:".secrets/token.json"`
}

func main() {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		log.Fatalf("config error: %v", err)
	}

	if err := os.MkdirAll(".secrets", 0700); err != nil {
		log.Fatalf("creating .secrets dir: %v", err)
	}

	if err := classroom.RunAuthFlow(context.Background(), cfg.ClientID, cfg.ClientSecret, cfg.TokenFile); err != nil {
		log.Fatalf("auth failed: %v", err)
	}
}
