package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/caarlos0/env/v11"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prokhorind/google-classroom-mcp/classroom"
	"github.com/prokhorind/google-classroom-mcp/tools"
)

// Config holds all runtime configuration sourced from environment variables.
type Config struct {
	// OAuth2 client credentials from Google Cloud Console.
	ClientID     string `env:"GOOGLE_CLIENT_ID,required"`
	ClientSecret string `env:"GOOGLE_CLIENT_SECRET,required"`

	// Path where the OAuth2 token is cached after the first auth flow.
	// Should be an absolute path when running as an MCP server.
	TokenFile string `env:"GOOGLE_TOKEN_FILE,required"`

	// Base directory where student submissions are saved.
	// Should be an absolute path when running as an MCP server.
	SubmissionsDir string `env:"SUBMISSIONS_DIR,required"`
}

func main() {
	ctx := context.Background()

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		log.Fatalf("config error: %v", err)
	}

	// Ensure token directory exists
	if err := os.MkdirAll(filepath.Dir(cfg.TokenFile), 0700); err != nil {
		log.Fatalf("creating token dir: %v", err)
	}

	svc, httpClient, err := classroom.NewService(ctx, cfg.ClientID, cfg.ClientSecret, cfg.TokenFile)
	if err != nil {
		log.Fatalf("failed to create classroom service: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "google-classroom-mcp",
		Version: "v1.0.0",
	}, nil)

	tools.Register(server, svc, httpClient, cfg.SubmissionsDir)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
