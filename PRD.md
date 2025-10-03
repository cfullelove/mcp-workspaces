# Product Requirements Document (PRD)
## Prototype MCP Workspace Manager (Go, stdio/HTTP SSE)

### Document version
- Version: 1.1 (Draft)
- Date: 2025-10-03
- Author: Assistant
- Stakeholder location context: Brisbane, QLD, Australia
- Language/locale: UK English

---

## 1. Overview

### 1.1 Summary
Build a standalone Go application (single binary) that implements a Model Context Protocol (MCP) server, configurable to run over either stdio or HTTP SSE. The server manages “workspaces”, which are directories under a configurable parent directory. It exposes a set of file and directory tools scoped to a given workspace. This is a prototype optimised for text-file operations but supports binary media reading for images/audio.

### 1.2 Goals
- Provide a minimal-yet-complete MCP server with:
  - Workspace lifecycle: create workspace.
  - Workspace-scoped file tools: read/write/edit/list/move/search/inspect.
  - Automatic Git-based versioning for all write operations within a workspace.
- Support both stdio and HTTP SSE transports behind a common abstraction.
- Enforce safe filesystem operations confined to the configured workspace root.
- Offer sensible defaults and simple configuration (flags and/or env vars).
- Keep design clean for future extension, but prioritise shipping a working prototype.

### 1.3 Non-goals
- Persistent database or user management.
- Advanced permission models or multi-tenant isolation beyond filesystem scoping.
- Large file optimisation, rate limiting, or comprehensive content-type handling.
- The “list_allowed_directories” tool (explicitly removed).

---

## 2. Users and Use Cases

### 2.1 Primary users
- Developers integrating MCP clients (e.g., IDE or agent frameworks) who need a local file/workspace backend with predictable behaviours over stdio or HTTP SSE.

### 2.2 Key use cases
- Create a new workspace directory under a known parent.
- Read, write, and edit files within a workspace, with changes automatically versioned.
- Inspect directory contents and metadata.
- Move/rename files or directories safely within a workspace.
- Search for files using glob-like patterns.
- Retrieve a structured directory tree for visualisation.
- Read media files for preview or processing.
- View the commit history of a file or an entire workspace.

---

## 3. Requirements

### 3.1 Functional requirements

#### 3.1.1 Transport and server
- The server shall support two transports:
  - stdio (MCP over stdio).
  - HTTP SSE (MCP over HTTP with Server-Sent Events) with a configurable port.
- The transport shall be selectable via CLI flag `--transport=stdio|http` or environment variable `MCP_TRANSPORT`.
- When `--transport=http`, the server shall listen on a configurable port via `--port` or `PORT` (default 8080).
- The server shall define a clear MCP tool registry and dispatch incoming MCP requests to tool handlers.
- The server shall validate request schemas and return structured error responses with codes and messages.

#### 3.1.2 Configuration
- The application shall be configured via:
  - `--workspaces-root` or `WORKSPACES_ROOT` (required).
  - `--transport=stdio|http` or `MCP_TRANSPORT` (required).
  - `--port` or `PORT` (optional; used only when `http`; default 8080).
  - Optional logging configuration: `--log-format=text|json` (default text), `--log-level=info|debug|warn|error` (default info).

#### 3.1.3 Workspace management
- Create workspace tool:
  - Input: `name` (string).
  - Behaviour:
    - Generate a workspace ID as a slug from `name`:
      - Lowercase normalisation.
      - Replace whitespace and invalid filesystem characters with `-`.
      - Collapse repeated `-`, trim leading/trailing `-`.
      - Truncate to a safe length (e.g., 64 chars) to avoid OS/path issues.
    - Ensure uniqueness under `workspaces-root`:
      - If the slug already exists, append `-n` or a short disambiguator (e.g., 4–6 char hash) and return the final slug.
    - Create a subdirectory under `workspaces-root` with the final slug.
    - Initialize a new Git repository within this directory.
  - Output: `{ workspaceId: string, path: string }`, where `path` is the absolute canonical path.

- All subsequent tool calls shall require `workspaceId` and operate relative to that workspace directory.

#### 3.1.4 Git Integration
- Upon creation, each workspace directory shall be initialized as a Git repository.
- All file system write operations (`write_file`, `edit_file`, `create_directory`, `move_file`) shall be automatically committed to the repository.
- Commit messages shall be descriptive and automatically generated, indicating the tool that triggered the commit (e.g., "mcp/fs/write_file: Create users/data.txt").

#### 3.1.5 Tools (per workspace)

For each tool, inputs must include `workspaceId` and workspace-relative `path`(s) unless explicitly noted.

- read_text_file
  - Inputs:
    - `path` (string): file path relative to workspace root.
    - `head` (number, optional): return first N lines.
    - `tail` (number, optional): return last N lines.
  - Constraints:
    - Treat file as UTF-8 text regardless of extension.
    - Cannot specify both `head` and `tail`. If both provided, return a validation error.
  - Output:
    - `{ content: string, head?: number, tail?: number, totalLines?: number }`
    - If `head` or `tail` is used, include `totalLines` if efficient to compute; otherwise omit or compute by scanning once.

- read_media_file
  - Inputs:
    - `path` (string).
  - Behaviour:
    - Stream the file and return base64-encoded data with appropriate MIME type.
    - MIME type detection via magic sniffing (preferred) or extension fallback.
  - Output:
    - `{ mimeType: string, base64: string, size: number }`
  - Errors:
    - If file too large for memory, return a clear error noting prototype limitation.

- read_multiple_files
  - Inputs:
    - `paths` (string[]): list of workspace-relative paths.
  - Behaviour:
    - Execute file reads concurrently with a bounded worker pool (e.g., up to CPU count).
    - Failures for specific files do not abort the entire operation.
  - Output:
    - `{ results: Array<{ path: string, ok: boolean, content?: string, error?: string }> }`
    - Treat all files as UTF-8 text; binary-safe read may be optionally gated but default to text.

- write_file
  - Inputs:
    - `path` (string).
    - `content` (string).
  - Behaviour:
    - Create or overwrite the file. Create parent directories as needed.
    - After a successful write, stage and commit the change.
  - Output:
    - `{ path: string, bytesWritten: number, overwritten: boolean, commit: string }`

- edit_file
  - Inputs:
    - `path` (string).
    - `edits` (array of `{ oldText: string, newText: string }`).
    - `dryRun` (boolean, default false).
  - Behaviour:
    - Apply search/replace operations in a deterministic order (e.g., input order).
    - Preserve indentation style when possible; normalise whitespace as needed.
    - When `dryRun = true`, do not write; return a Git-style unified diff plus match stats.
    - When `dryRun = false`, after applying changes, stage and commit them.
  - Output:
    - Dry run: `{ dryRun: true, diff: string, matches: number }`
    - Applied: `{ dryRun: false, path: string, changes: number, bytesWritten: number, commit: string }`
  - Notes:
    - Prototype scope: substring replacement; no regex required, but structure allows extension.

- create_directory
  - Inputs:
    - `path` (string).
  - Behaviour:
    - Create directory; create parents; succeed if exists (idempotent).
    - After creating the directory, stage and commit the change (e.g., by adding a `.gitkeep` file if the directory is empty, to ensure it's tracked).
  - Output:
    - `{ path: string, created: boolean, commit: string }`

- list_directory
  - Inputs:
    - `path` (string).
  - Behaviour:
    - List entries with prefixes “[FILE] ” or “[DIR] ”.
  - Output:
    - `{ entries: string[] }` where each entry is prefixed.

- list_directory_with_sizes
  - Inputs:
    - `path` (string).
    - `sortBy` (string, optional: "name" | "size"; default "name").
  - Behaviour:
    - Include per-entry size (files) and potentially computed directory sizes if efficient; otherwise show directory entries with size 0 or omit directory size as a prototype simplification.
  - Output:
    - `{ entries: Array<{ name: string, type: 'file'|'directory', size: number }>, totals: { files: number, directories: number, combinedSize: number } }`

- move_file
  - Inputs:
    - `source` (string), `destination` (string).
  - Behaviour:
    - Move/rename within workspace; fail if destination exists.
    - After a successful move, stage and commit the change (`git add -A` and `git commit`).
  - Output:
    - `{ source: string, destination: string, commit: string }`

- search_files
  - Inputs:
    - `path` (string) starting directory.
    - `pattern` (string) glob-like.
    - `excludePatterns` (string[]).
  - Behaviour:
    - Recursive search using glob matching; apply excludes.
  - Output:
    - `{ matches: string[] }` (workspace-relative paths).

- directory_tree
  - Inputs:
    - `path` (string) starting directory.
    - `excludePatterns` (string[]).
  - Behaviour:
    - Produce JSON tree with 2-space indentation.
    - For files: no `children` field.
    - For empty directories: `children: []`.
  - Output:
    - `{ tree: Array<{ name: string, type: 'file'|'directory', children?: any[] }> }`

- get_file_info
  - Inputs:
    - `path` (string).
  - Behaviour:
    - Return metadata: size, creation time, modified time, access time, type, permissions.
    - Note: creation time may be platform-dependent; provide best effort and document limitations.
  - Output:
    - `{ size: number, ctime?: string, mtime: string, atime?: string, type: 'file'|'directory', permissions: string }`

- get_commit_history
  - Inputs:
    - `workspaceId` (string).
    - `path` (string, optional): file path to get history for.
    - `limit` (number, optional, default 20): number of commits to return.
  - Behaviour:
    - Return the git log for the specified path or the entire workspace.
  - Output:
    - `{ log: Array<{ commit: string, author: string, date: string, message: string }> }`

- Removed tool
  - list_allowed_directories: not implemented.

#### 3.1.6 Path Handling and Safety
- Resolve all input paths against the canonical workspace root.
- Reject operations that would escape the workspace (e.g., via `..` traversal or symlink resolution leading outside root).
- Normalise path separators for Windows and Linux appropriately.
- Return clear errors for invalid paths.

#### 3.1.7 Error handling
- Use a consistent error schema:
  - `{ code: string, message: string, details?: any }`
- Common codes:
  - `INVALID_INPUT`, `NOT_FOUND`, `ALREADY_EXISTS`, `PERMISSION_DENIED`, `OUT_OF_BOUNDS`, `UNSUPPORTED`, `INTERNAL`.
- For `read_text_file`, if both `head` and `tail` are provided, return `INVALID_INPUT`.
- For `move_file`, if destination exists, return `ALREADY_EXISTS`.

#### 3.1.8 Logging and observability
- Logging:
  - Log server start, transport, port, workspaces root.
  - Log tool invocations at debug level with redacted paths where necessary.
  - Log errors with stack/context at error level.
- Formats:
  - `--log-format=text|json` (default text).
  - Levels as in 3.1.2.

---

## 4. Protocol and API

### 4.1 MCP framing
- The server shall register tools and expose them via MCP-compliant request/response messages for both transports.
- For HTTP SSE:
  - Provide an endpoint to establish SSE stream (e.g., `GET /mcp/stream`) and a complementary POST endpoint for commands if required by the chosen library or internal design.
  - Ensure CORS can be toggled off/on if needed (prototype default: permissive for local development).
- For stdio:
  - Read JSON messages from stdin and write responses to stdout, following MCP message envelopes (IDs, method/tool name, params, result/error).

### 4.2 Tool naming
- Tools should be named clearly; suggested names:
  - `workspace/create`
  - `fs/read_text_file`
  - `fs/read_media_file`
  - `fs/read_multiple_files`
  - `fs/write_file`
  - `fs/edit_file`
  - `fs/create_directory`
  - `fs/list_directory`
  - `fs/list_directory_with_sizes`
  - `fs/move_file`
  - `fs/search_files`
  - `fs/directory_tree`
  - `fs/get_file_info`
  - `fs/get_commit_history`

### 4.3 Versioning
- Include an `x-server` info response tool (optional) returning `{ name, version, transport }` for diagnostics.
- Semantic version the server binary (e.g., `0.1.0` for the prototype).

---

## 5. UX and Developer Experience

### 5.1 CLI UX
- Example usage:
  - `app --transport=http --port=8080 --workspaces-root=/data/workspaces`
  - `app --transport=stdio --workspaces-root=C:\workspaces`
- Helpful errors if required flags are missing, with examples.

### 5.2 Responses
- Keep responses small, structured, and predictable.
- Use workspace-relative paths in results whenever practical; include absolute path only when explicitly part of output (e.g., create workspace returns both).

---

## 6. Performance and Scalability

- Concurrency:
  - Use goroutines for parallel reads in `read_multiple_files`; bound with a worker pool (default size: min(8, GOMAXPROCS*2), configurable later if needed).
- I/O:
  - Stream when possible for media reads, then buffer to base64; note that very large media may exhaust memory in the prototype.
- Filesystem operations are local; network filesystems not specifically optimised.

---

## 7. Security and Privacy

- Filesystem sandboxing:
  - Enforce workspace root boundaries; resolve symlinks and canonical paths before operations.
- Network:
  - Prototype HTTP server can run without TLS; recommend deployment behind a secure proxy if exposed.
- Input validation:
  - Validate tool parameters strictly; reject unsafe names/paths.
- Logging:
  - Avoid logging full file contents; redact sensitive data.

---

## 8. Platform and Compatibility

- Language: Go (1.21+ recommended).
- OS targets: Windows and Linux.
- Build: single static binary where practical; standard `go build`.
- Path handling: use `filepath` for OS-specific separators; ensure normalisation for Windows drive letters.

---

## 9. Dependencies and Libraries

- MCP transport:
  - Prefer an existing Go MCP library that supports stdio and HTTP SSE. If none is mature, implement a minimal MCP transport abstraction with:
    - stdio: JSON-RPC-like messages over stdin/stdout.
    - HTTP SSE: Gorilla/standard library HTTP with SSE writer plus a POST command channel, or a single duplex if the chosen library supports it.
- Git Integration:
  - `go-git/go-git` or a similar library for programmatic Git operations.
- MIME detection:
  - `net/http`’s `DetectContentType` plus extension map fallback.
- Logging:
  - `log/slog` (standard) or a lightweight structured logger.

Note: Prior to implementation, verify availability of a Go MCP library providing both transports. If available, adopt to reduce boilerplate and increase interoperability.

---

## 10. Acceptance Criteria

- Configuration
  - Running with `--transport=http --port=8080 --workspaces-root=./data` starts an HTTP SSE MCP server without errors.
  - Running with `--transport=stdio --workspaces-root=./data` starts an MCP stdio server on stdin/stdout.

- Workspace creation
  - Given name “My First Workspace”, server returns `workspaceId: "my-first-workspace"`, creates `./data/my-first-workspace`, and initializes a git repository inside it.
  - If “My First Workspace” is created again, a unique variant is returned (e.g., `my-first-workspace-2` or `my-first-workspace-ab12`).

- File tools
  - `write_file` successfully creates or overwrites a file and creates a corresponding git commit with a descriptive message. The commit hash is returned in the response.
  - `edit_file` with `dryRun: false` applies changes and creates a commit. The commit hash is returned.
  - `move_file` successfully moves a file and creates a commit. The commit hash is returned.
  - `create_directory` successfully creates a directory and creates a commit. The commit hash is returned.
  - `get_commit_history` returns a list of commits for the workspace or a specific file.
  - `read_text_file` returns full contents; with `head: 5`, returns first 5 lines; specifying both `head` and `tail` yields `INVALID_INPUT`.
  - `read_media_file` returns base64 and correct MIME for a PNG or MP3.
  - `read_multiple_files` returns mixed success without failing overall if one path is missing.
  - `edit_file` with `dryRun: true` produces a diff; with `dryRun: false` applies changes and returns stats.
  - `list_directory` shows “[FILE] ” and “[DIR] ” prefixes.
  - `list_directory_with_sizes` returns entries with sizes and totals; sorted by name by default, by size when requested.
  - `search_files` returns matches consistent with provided glob and excludes.
  - `directory_tree` returns structure with 2-space indentation rules and correct `children` semantics.
  - `get_file_info` includes type and permissions; notes platform limitations for `ctime` and `atime` if absent.

- Safety
  - Attempts to access `../../` escape the workspace are rejected with `OUT_OF_BOUNDS`.
  - Symlink that points outside workspace is rejected.

- Logging
  - Startup logs show transport, port (if http), and workspaces root.
  - Errors include code and message.

---

## 11. Risks and Mitigations

- Risk: No mature Go MCP library for both stdio and HTTP SSE.
  - Mitigation: Implement a lightweight transport abstraction; start with stdio (simpler), then add HTTP SSE using standard library.

- Risk: Platform differences in file timestamps and permissions.
  - Mitigation: Document fields as best-effort; include presence-guarded fields.

- Risk: Large media files cause memory pressure during base64 encoding.
  - Mitigation: Document prototype limitation; consider size checks and early warnings.

- Risk: Path normalisation mistakes on Windows.
  - Mitigation: Use `filepath` consistently; extensive unit tests with edge cases.

- Risk: Git operations could be slow and block requests.
  - Mitigation: For this prototype, all operations are synchronous. Future enhancements could make git operations asynchronous.

---

## 12. Milestones

- M1: Project scaffold
  - CLI flags/env, logging, config validation, transport interface.

- M2: stdio transport + core routing
  - Register tools, implement request/response envelopes.

- M3: Workspace create + Git Integration
  - Update `workspace/create` to initialize a git repo.
  - Implement a git wrapper for staging and committing changes.

- M4: Core FS tools with Git
  - `write_file`, `edit_file`, `create_directory`, `move_file` with automatic commits.
  - `read_text_file`, `list_directory`, `get_file_info`.

- M5: Advanced tools
  - `search_files`, `directory_tree`, `list_directory_with_sizes`, `read_multiple_files`.
  - `get_commit_history`.

- M6: Media reading
  - `read_media_file` with MIME detection and base64.

- M7: HTTP SSE transport
  - SSE stream endpoint, command handling, parity with stdio.

- M8: Hardening and tests
  - Path safety, symlink escape checks, error codes, concurrency tests, git integration tests.

---

## 13. Testing Strategy

- Unit tests:
  - Slug generation, path normalisation, workspace scoping.
  - Each tool’s happy-path and failure modes.
  - Git commit generation and history retrieval.
- Integration tests:
  - End-to-end flows over stdio and HTTP, checking for correct file state and git commits after each operation.
- Manual tests:
  - Large file behaviours, binary vs text reading, Windows path scenarios.

---

## 14. Future Enhancements (Out of Scope)

- Authentication/authorisation for HTTP mode.
- Configurable concurrency and file size limits.
- Binary-safe multiple read mode with MIME hints.
- Regex support and richer edit operations.
- Workspace deletion and archival.
- Observability: metrics, tracing.
- Pluggable storage backends (e.g., S3, SMB).
- Git branching, merging, and reverting capabilities.

---

## 15. Glossary

- MCP: Model Context Protocol; a protocol for tool registration and invocation between a client and server.
- stdio: Standard input/output stream communication.
- HTTP SSE: Server-Sent Events over HTTP for pushing events from server to client.