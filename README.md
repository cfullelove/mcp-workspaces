# MCP Workspace Manager (Go, MCP SDK)

A standalone Go application implementing an MCP server for managing sandboxed “workspaces” (directories under a configured root). Exposes file and directory tools with automatic Git commits for mutations.

This implementation uses the official Model Context Protocol Go SDK for transports and protocol compliance.

## Features

- Transports (via MCP SDK)
  - stdio
  - HTTP Streamable endpoint: /mcp
  - HTTP SSE endpoint: /mcp/sse
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
- SSE:        http://HOST:PORT/mcp/sse
- Health:     http://HOST:PORT/healthz

Add to Claude Code (streamable):

```bash
claude mcp add -t http workspaces http://localhost:8080
```

## Testing

Integration tests cover stdio, Streamable HTTP, and SSE.

Run tests:

```bash
go test -v ./...
```

What tests do:

- Build the current binary
- Start the server for the chosen transport
- Use MCP SDK clients (CommandTransport, StreamableClientTransport, SSEClientTransport)
- Call workspace_create and verify workspace on disk

## Tool Behavior Notes

- fs_read_text_file: mutually exclusive head/tail; returns totalLines when efficient
- fs_search_files: prototype name-glob match with excludes on file names
- fs_create_directory: idempotent, ensures empty directories tracked with .gitkeep
- fs_edit_file: substring replace prototype; dryRun returns a unified-style diff

## Security & Limits

- Local filesystem operations only; path traversal is blocked by SafePath
- HTTP endpoints are not authenticated; for local development only
- Streamable HTTP supports session resumption
- Media files are limited to 10MB in this prototype

## License

MIT for this project. The MCP SDK is licensed separately under its respective license.
