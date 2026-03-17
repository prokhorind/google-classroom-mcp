package main

import (
	"context"
	"encoding/json"
	"github.com/caarlos0/env/v11"
	"github.com/prokhorind/google-classroom-mcp/classroom"
	"log"
	"os"
)

// Config holds all runtime configuration sourced from environment variables.
type Config struct {
	// Path to credentials.json downloaded from Google Cloud Console.
	CredentialsFile string `env:"GOOGLE_CREDENTIALS_FILE" envDefault:".secrets/credentials.json"`

	// Path where the OAuth2 token is cached after the first auth flow.
	TokenFile string `env:"GOOGLE_TOKEN_FILE" envDefault:".secrets/token.json"`

	// Base directory where student submissions are saved.
	SubmissionsDir string `env:"SUBMISSIONS_DIR" envDefault:"./submissions"`
}

func main() {
	ctx := context.Background()

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		log.Fatalf("config error: %v", err)
	}

	if err := os.MkdirAll(".secrets", 0700); err != nil {
		log.Fatalf("create .secrets dir: %v", err)
	}

	svc, _, err := classroom.NewService(ctx, cfg.CredentialsFile, cfg.TokenFile)
	if err != nil {
		log.Fatalf("failed to create classroom service: %v", err)
	}

	courses, err := classroom.ListCourses(ctx, svc)

	if err != nil {
		log.Fatalf("listing courses: %v", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(courses)

	// --- MCP server (commented out for API testing) ---
	// server := mcp.NewServer(&mcp.Implementation{
	// 	Name:    "google-classroom-mcp",
	// 	Version: "v1.0.0",
	// }, nil)
	// tools.Register(server, svc, httpClient, cfg.SubmissionsDir)
	// if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
	// 	log.Fatalf("server error: %v", err)
	// }
}
