package mcpsdk

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-workspace-manager/pkg/events"
	"mcp-workspace-manager/pkg/workspace"
)

// RunHTTP serves the MCP SDK server over HTTP using the Streamable HTTP transport,
// and exposes a REST mirror of the tools under /api/tools/{toolName}.
// If authTokens is non-empty, Bearer auth is required for /mcp*, /api/* endpoints.
func RunHTTP(host string, port int, wm *workspace.Manager, authTokens []string, rootHandler http.Handler) {
	server := buildServer(wm)

	// Create a streamable HTTP handler (supports resumption and reliable streaming).
	streamable := sdkmcp.NewStreamableHTTPHandler(func(r *http.Request) *sdkmcp.Server {
		return server
	}, nil)

	mux := http.NewServeMux()

	// Initialize global event hub and mount SSE endpoint for browsers
	// Note: Authorization for /events is handled by the SSE handler (query token or Bearer).
	eventHub = events.NewHub(200)
	mux.Handle("/events", events.SSEHandler(eventHub, authTokens))

	// Start filesystem watcher to capture external changes (not via API/MCP)
	if stopFn, err := events.StartFSWatcher(wm.RootPath(), eventHub); err != nil {
		slog.Warn("Failed to start fs watcher", "error", err)
	} else {
		_ = stopFn // kept for future graceful shutdown
	}

	// Protected mounts (streamable and SSE alias)
	protected := []struct {
		pattern string
		h       http.Handler
	}{
		{"/mcp", streamable},
		{"/mcp/stream", streamable},
		{"/mcp/command", streamable},
		// SSE compatibility mount to streamable (SDK v0.4.0 may not expose SSE handler)
		{"/mcp/sse", streamable},
		// REST tools mirror
		{"/api/tools/", restToolsHandler(wm)},
	}
	for _, p := range protected {
		mux.Handle(p.pattern, wrapAuth(p.h, authTokens))
	}

	// Health probe (unauthenticated)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Serve the embedded frontend
	if rootHandler != nil {
		mux.Handle("/", rootHandler)
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	slog.Info("Starting MCP SDK HTTP server", "host", host, "port", port, "addr", addr, "auth_enabled", len(authTokens) > 0)

	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("MCP SDK HTTP server failed", "error", err)
	}
}

// wrapAuth applies simple Bearer token auth when tokens is non-empty.
// Authorization: Bearer <token> (case-insensitive "Bearer").
// On failure: 401 with WWW-Authenticate header.
func wrapAuth(next http.Handler, tokens []string) http.Handler {
	// Disabled if no tokens
	if len(tokens) == 0 {
		return next
	}
	// Normalize and deduplicate tokens
	norm := make([][]byte, 0, len(tokens))
	seen := map[string]struct{}{}
	for _, t := range tokens {
		tt := strings.TrimSpace(t)
		if tt == "" {
			continue
		}
		if _, ok := seen[tt]; ok {
			continue
		}
		seen[tt] = struct{}{}
		norm = append(norm, []byte(tt))
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if authz == "" {
			unauthorized(w)
			return
		}
		// Expect "Bearer token" with case-insensitive scheme.
		var token string
		if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			token = strings.TrimSpace(authz[len("Bearer "):])
		} else {
			parts := strings.SplitN(authz, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				token = strings.TrimSpace(parts[1])
			}
		}
		if token == "" {
			unauthorized(w)
			return
		}

		got := []byte(token)
		ok := false
		for _, allowed := range norm {
			if subtle.ConstantTimeCompare(got, allowed) == 1 {
				ok = true
				break
			}
		}
		if !ok {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="mcp", error="invalid_token"`)
	http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
}

// REST mirror: POST /api/tools/{toolName}
func restToolsHandler(wm *workspace.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		toolName := strings.TrimPrefix(r.URL.Path, "/api/tools/")
		if toolName == "" || strings.Contains(toolName, "/") {
			http.NotFound(w, r)
			return
		}

		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		w.Header().Set("Content-Type", "application/json")

		var err error
		switch toolName {

		case "workspace_create":
			var in CreateWorkspaceRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := WorkspaceCreate(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_delete_file":
			var in DeleteFileRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSDeleteFile(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "workspace_list":
			var in ListWorkspacesRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := WorkspaceList(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_write_file":
			var in WriteFileRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSWriteFile(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_read_text_file":
			var in ReadFileRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSReadTextFile(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_create_directory":
			var in CreateDirectoryRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSCreateDirectory(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_list_directory":
			var in ListDirectoryRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSListDirectory(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_get_file_info":
			var in GetFileInfoRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSGetFileInfo(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_get_commit_history":
			var in GetCommitHistoryRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSGetCommitHistory(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_move_file":
			var in MoveFileRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSMoveFile(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_edit_file":
			var in EditFileRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSEditFile(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_read_multiple_files":
			var in ReadMultipleFilesRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSReadMultipleFiles(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_list_directory_with_sizes":
			var in ListDirectoryWithSizesRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSListDirectoryWithSizes(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_search_files":
			var in SearchFilesRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSSearchFiles(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_directory_tree":
			var in DirectoryTreeRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSDirectoryTree(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_read_media_file":
			var in ReadMediaFileRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSReadMediaFile(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		case "fs_read_file_at_commit":
			var in ReadFileAtCommitRequest
			if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeRESTError(w, errBadRequest(err))
				return
			}
			out, e := FSReadFileAtCommit(r.Context(), wm, in)
			if e != nil {
				writeRESTError(w, e)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = enc.Encode(out)

		default:
			http.NotFound(w, r)
			return
		}
	})
}

func writeRESTError(w http.ResponseWriter, err error) {
	code := httpStatusFromError(err)
	http.Error(w, err.Error(), code)
}

func errBadRequest(err error) error {
	return &restErr{msg: "INVALID_INPUT: " + err.Error()}
}

type restErr struct{ msg string }

func (e *restErr) Error() string { return e.msg }

func httpStatusFromError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "INVALID_INPUT:"):
		return http.StatusBadRequest
	case strings.HasPrefix(msg, "NOT_FOUND:"):
		return http.StatusNotFound
	case strings.HasPrefix(msg, "ALREADY_EXISTS:"):
		return http.StatusConflict
	case strings.HasPrefix(msg, "CONFLICT:"):
		return http.StatusConflict
	case strings.HasPrefix(msg, "OUT_OF_BOUNDS:"):
		return http.StatusBadRequest
	case strings.HasPrefix(msg, "UNSUPPORTED:"):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}
