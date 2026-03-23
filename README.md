# google-classroom-mcp

> ⚠️ Work in progress

MCP server for grading Google Classroom student submissions locally using an AI agent.

## Setup

1. Go to [Google Cloud Console](https://console.cloud.google.com/apis/credentials) and create an OAuth2 client ID with type **Desktop app**
2. Download `credentials.json` and place it in `.secrets/credentials.json`
3. Enable the **Google Classroom API** and **Google Drive API** in your project

## First run

On first run the server will open a browser for OAuth2 consent and cache the token to `.secrets/token.json`.

```bash
go run main.go
```

## Test with MCP Inspector

```bash
npx @modelcontextprotocol/inspector go run main.go
```

Opens at `http://localhost:5173`.

## Use in Kiro

Add to `.kiro/settings/mcp.json`.

Option 1 — run directly with Go (no build needed):

```json
{
  "mcpServers": {
    "google-classroom-mcp": {
      "command": "go",
      "args": ["run", "/absolute/path/to/google-classroom-mcp/main.go"]
    }
  }
}
```

Option 2 — build first, then point to the binary:

```bash
go build -o classroom-mcp .
```

```json
{
  "mcpServers": {
    "google-classroom-mcp": {
      "command": "/absolute/path/to/classroom-mcp"
    }
  }
}
```

## Tools

- `get_submissions` — resolves course/task by name, downloads student files, returns paths + reference answer for grading
- `save_grade` — persists a grade and feedback locally to `grades.json`

## Env vars

| Variable | Default | Description |
|---|---|---|
| `GOOGLE_CREDENTIALS_FILE` | `.secrets/credentials.json` | Path to OAuth2 credentials |
| `GOOGLE_TOKEN_FILE` | `.secrets/token.json` | Path to cached token |
| `SUBMISSIONS_DIR` | `./submissions` | Where student files are saved |
