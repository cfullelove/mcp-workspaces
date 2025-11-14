//go:build !dev

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTP_EmbeddedFrontend(t *testing.T) {
	// Build the binary
	tmpBinDir, err := os.MkdirTemp("", "mcp-ws-bin-embed")
	require.NoError(t, err)
	defer os.RemoveAll(tmpBinDir)

	binaryPath := filepath.Join(tmpBinDir, "mcp-workspace-manager")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}

	// Build the Go binary
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Env = os.Environ()
	out, err := build.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(out))

	// Prepare workspace root and start server
	wsRoot, err := os.MkdirTemp("", "mcp-ws-root-embed")
	require.NoError(t, err)
	defer os.RemoveAll(wsRoot)

	host := "127.0.0.1"
	port := "18083"
	server := exec.Command(binaryPath, "--transport=http", "--host="+host, "--port="+port, "--workspaces-root="+wsRoot)
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	require.NoError(t, server.Start())
	defer func() {
		_ = server.Process.Kill()
	}()

	// Give server time to bind
	time.Sleep(750 * time.Millisecond)

	// Make request to the root
	endpoint := fmt.Sprintf("http://%s:%s/", host, port)
	resp, err := http.Get(endpoint)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Check the response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(body), "<title>frontend</title>"), "response should contain the title")
}
