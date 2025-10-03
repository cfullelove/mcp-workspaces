package mcpsdk

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-workspace-manager/pkg/workspace"
)

// RunHTTP serves the MCP SDK server over HTTP using the Streamable HTTP transport.
// It mounts the handler at multiple paths for compatibility:
//
// - /mcp           (primary mount)
// - /mcp/stream    (compat shim)
// - /mcp/command   (compat shim)
//
// A permissive /healthz endpoint is also provided.
func RunHTTP(host string, port int, wm *workspace.Manager) {
	server := buildServer(wm)

	// Create a streamable HTTP handler (supports resumption and reliable streaming).
	streamable := sdkmcp.NewStreamableHTTPHandler(func(r *http.Request) *sdkmcp.Server {
		return server
	}, nil)

	mux := http.NewServeMux()
	// Primary mount
	mux.Handle("/mcp", streamable)

	// Basic compatibility mounts (forward to same handler)
	mux.Handle("/mcp/stream", streamable)
	mux.Handle("/mcp/command", streamable)

	// SSE compatibility mount to streamable (SDK v0.4.0 may not expose SSE handler)
	// If SDK adds SSEHTTPHandler in a future version, replace with the real SSE handler.
	mux.Handle("/mcp/sse", streamable)

	// Health probe
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	slog.Info("Starting MCP SDK HTTP server", "host", host, "port", port, "addr", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("MCP SDK HTTP server failed", "error", err)
	}
}
