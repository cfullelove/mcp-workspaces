# MCP Workspace Manager (Go, MCP SDK)

A standalone Go application implementing an MCP server for managing sandboxed “workspaces” (directories under a configured root). Exposes file and directory tools with automatic Git commits for mutations.

This implementation uses the official Model Context Protocol Go SDK for transports and protocol compliance.

## Features

- Transports (via MCP SDK)
  - stdio
  - HTTP Streamable endpoint: /mcp
  - HTTP SSE endpoint: /mcp/sse (compat alias to streamable until SDK exposes SSE server)
- REST API
  - 1:1 mirror of MCP tools at: POST /api/tools/{toolName}
- Authentication
  - Optional Bearer token auth for HTTP endpoints (/mcp*, /api/*). Multiple tokens supported.
- Tools (workspace-scoped)
  - workspace_create
  - fs_write_file
  - fs_read_text_file
  - fs_create_directory
  - fs_list_directory
  - fs_get_file_info
  - fs_get_commit_history
  - fs_move_file
  - fs_edit_file
  - fs_read_multiple_files
  - fs_list_directory_with_sizes
  - fs_search_files
  - fs_directory_tree
  - fs_read_media_file
- Git integration: mutations commit with descriptive messages
- Path safety: operations are confined to the workspace root
- Logging: structured (text/json) with selectable levels

## Build

Requires Go 1.21+ (recommended 1.24+).

```bash
go build -o mcp-workspace-manager .
```

## Run

You must set a workspace root. Transports are selectable via flag or env.

- workspaces root:
  - flag: --workspaces-root=/path/to/workspaces
  - env: WORKSPACES_ROOT
- transport:
  - flag: --transport=stdio|http
  - env: MCP_TRANSPORT
- when transport=http:
  - host:
    - flag: --host=127.0.0.1
    - env: HOST
    - default: 127.0.0.1
  - port:
    - flag: --port=8080
    - env: PORT
    - default: 8080
  - authentication (optional; if omitted, HTTP endpoints are open)
    - flag: --auth-tokens="tokA,tokB,..." (comma-separated, multi-token)
    - flag: --auth-token="singleToken" (back-compat; appended if provided)
    - env: AUTH_BEARER_TOKENS="tokA,tokB,..."
    - env: AUTH_BEARER_TOKEN="singleToken"
    - Behavior: If any token is configured, all /mcp*, /api/* endpoints require `Authorization: Bearer <token>` matching one of the configured tokens. `/healthz` remains unauthenticated.
- logging:
  - --log-format=text|json (default text)
  - --log-level=debug|info|warn|error (default info)

### Examples

Stdio:

```bash
./mcp-workspace-manager --transport=stdio --workspaces-root=./data
```

HTTP Streamable (primary) on custom host/port:

```bash
./mcp-workspace-manager --transport=http --host=0.0.0.0 --port=9000 --workspaces-root=./data
```

HTTP endpoints:

- Streamable: http://HOST:PORT/mcp
- SSE (compat alias): http://HOST:PORT/mcp/sse
- REST (tools mirror): http://HOST:PORT/api/tools/{toolName}
- Health: http://HOST:PORT/healthz

Add to Claude Code (streamable):

```bash
claude mcp add -t http workspaces http://localhost:8080
```

## REST API (MCP Tool Mirror)

- Method: POST
- Path: /api/tools/{toolName}
- Request body: JSON matching the corresponding MCP tool input struct
- Response body: JSON matching the corresponding MCP tool output struct
- Error mapping (plain text body with HTTP status):
  - `INVALID_INPUT:` -> 400
  - `NOT_FOUND:` -> 404
  - `ALREADY_EXISTS:` -> 409
  - `OUT_OF_BOUNDS:` -> 400
  - `UNSUPPORTED:` -> 422
  - otherwise -> 500

Example: Create a workspace (no auth configured)

```bash
curl -sS -X POST http://127.0.0.1:8080/api/tools/workspace_create \
  -H 'Content-Type: application/json' \
  -d '{"name":"My REST Workspace"}'
# -> {"workspaceId":"my-rest-workspace","path":"/abs/path/to/workspaces/my-rest-workspace"}
```

Example: With Bearer auth

```bash
# Start server with tokens
./mcp-workspace-manager --transport=http --workspaces-root=./data --auth-tokens="tokA123,tokB456"

# Missing/invalid token -> 401
curl -i -X POST http://127.0.0.1:8080/api/tools/workspace_create \
  -H 'Content-Type: application/json' \
  -d '{"name":"My Secured Workspace"}'

# Correct token -> 200
curl -sS -X POST http://127.0.0.1:8080/api/tools/workspace_create \
  -H 'Authorization: Bearer tokA123' \
  -H 'Content-Type: application/json' \
  -d '{"name":"My Secured Workspace"}'
# -> {"workspaceId":"my-secured-workspace","path":"..."}
```

Another REST example: write a file

```bash
curl -sS -X POST http://127.0.0.1:8080/api/tools/fs_write_file \
  -H 'Content-Type: application/json' \
  -d '{
    "workspaceId":"my-rest-workspace",
    "path":"README.txt",
    "content":"hello"
  }'
# -> {"path":"README.txt","bytesWritten":5,"overwritten":false,"commit":"<hash>"}
```

## Authentication

- When at least one token is configured via flags/env, all HTTP endpoints under `/mcp`, `/mcp/stream`, `/mcp/command`, `/mcp/sse`, and `/api/*` require `Authorization: Bearer <token>`.
- Case-insensitive `Bearer` scheme; constant-time comparison against the configured token set.
- Multiple tokens supported. `/healthz` is always open.

## Testing

Integration tests cover:
- stdio
- Streamable HTTP
- REST with and without auth
- SSE test is skipped (SDK currently lacks server-side SSE handler)

Run tests:

```bash
go test -v ./...
```

What tests do:
- Build the current binary
- Start the server for the chosen transport
- Use MCP SDK clients (CommandTransport, StreamableClientTransport)
- Call workspace_create and verify workspace on disk
- Call REST mirror and verify behavior with and without Authorization

## Tool Behavior Notes

- fs_read_text_file: mutually exclusive head/tail; returns totalLines when efficient
- fs_search_files: prototype name-glob match with excludes on file names
- fs_create_directory: idempotent, ensures empty directories tracked with .gitkeep
- fs_edit_file: substring replace prototype; dryRun returns a diff

## Security & Limits

- Local filesystem operations only; path traversal is blocked by SafePath
- HTTP endpoints are unauthenticated by default; enable Bearer auth with flags/env as needed
- Streamable HTTP supports session resumption
- Media files are limited to 10MB in this prototype

## License

MIT for this project. The MCP SDK is licensed separately under its respective license.
