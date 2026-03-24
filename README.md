# google-classroom-mcp

> ⚠️ Work in progress

MCP server for grading Google Classroom student submissions locally using an AI agent.

## Setup

1. Go to [Google Cloud Console](https://console.cloud.google.com/apis/credentials) and create an OAuth2 client ID with type **Desktop app**
2. Enable the **Google Classroom API** and **Google Drive API** in your project
3. Copy the `client_id` and `client_secret` from the created credential

## Authentication (one-time)

Run the auth flow once to generate a cached token. Browser opens automatically, approve access, done.

```bash
GOOGLE_CLIENT_ID="your-client-id" \
GOOGLE_CLIENT_SECRET="your-client-secret" \
GOOGLE_TOKEN_FILE="/absolute/path/to/.secrets/token.json" \
go run ./cmd/auth
```

Token auto-refreshes — you never need to do this again unless you delete the token file.

## Kiro MCP config

Add to `~/.kiro/settings/mcp.json`:

```json
{
  "mcpServers": {
    "google-classroom-mcp": {
      "command": "/absolute/path/to/google-classroom-mcp/run.sh",
      "env": {
        "GOOGLE_CLIENT_ID": "your-client-id",
        "GOOGLE_CLIENT_SECRET": "your-client-secret",
        "GOOGLE_TOKEN_FILE": "/absolute/path/to/.secrets/token.json",
        "SUBMISSIONS_DIR": "/absolute/path/to/submissions"
      }
    }
  }
}
```

`run.sh` builds the binary and starts the server automatically — no manual build step needed.

## Test with MCP Inspector

```bash
GOOGLE_CLIENT_ID="..." GOOGLE_CLIENT_SECRET="..." \
GOOGLE_TOKEN_FILE="/absolute/path/to/.secrets/token.json" \
SUBMISSIONS_DIR="/absolute/path/to/submissions" \
npx @modelcontextprotocol/inspector ./run.sh
```

## Tools

- `get_submissions` — resolves course/task by name, downloads all student files locally, returns paths for grading

## Env vars

| Variable | Required | Description |
|---|---|---|
| `GOOGLE_CLIENT_ID` | yes | OAuth2 client ID from Google Cloud Console |
| `GOOGLE_CLIENT_SECRET` | yes | OAuth2 client secret |
| `GOOGLE_TOKEN_FILE` | yes | Absolute path to cached token (created by auth flow) |
| `SUBMISSIONS_DIR` | yes | Absolute path where student files are saved |

## Grading

Load the grading rules steering file in Kiro with `#grading-rules` then:

> grade submissions from course "Databases" task "Stored Procedures" using answers/stored-procedures.sql
