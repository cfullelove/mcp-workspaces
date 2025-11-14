package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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

// Minimal shape for events coming from /events SSE
type sseWorkspaceEvent struct {
	ID          int64  `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Type        string `json:"type"`
	Path        string `json:"path"`
	IsDir       bool   `json:"isDir"`
}

// Helpers

func buildBinary(t *testing.T) string {
	tmpBinDir, err := os.MkdirTemp("", "mcp-ws-bin-events")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpBinDir) })

	binaryPath := filepath.Join(tmpBinDir, "mcp-workspace-manager")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}
	build := exec.Command("go", "build", "-tags=dev", "-o", binaryPath, ".")
	build.Env = os.Environ()
	out, err := build.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(out))
	return binaryPath
}

func startServer(t *testing.T, binPath, wsRoot, host, port string, args ...string) *exec.Cmd {
	base := []string{
		"--transport=http",
		"--host=" + host,
		"--port=" + port,
		"--workspaces-root=" + wsRoot,
	}
	base = append(base, args...)
	server := exec.Command(binPath, base...)
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	require.NoError(t, server.Start())
	t.Cleanup(func() { _ = server.Process.Kill() })
	time.Sleep(750 * time.Millisecond) // allow bind
	return server
}

func restPOST(t *testing.T, url string, body any) *http.Response {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(http.MethodPost, url, rd)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func mustJSON[T any](t *testing.T, r io.Reader, out *T) {
	b, _ := io.ReadAll(r)
	require.NoError(t, json.Unmarshal(b, out), "body=%s", string(b))
}

func openSSE(t *testing.T, url string) (*http.Response, *bufio.Reader) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return resp, bufio.NewReader(resp.Body)
}

// readNextWorkspaceEvent parses the next complete SSE event with event: workspace.event
// It skips comment/heartbeat lines and unrelated events. Returns parsed event or (nil, io.EOF) if the stream closed.
func readNextWorkspaceEvent(rd *bufio.Reader, timeout time.Duration) (*sseWorkspaceEvent, error) {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return nil, context.DeadlineExceeded
		}
		// Read one SSE frame (until blank line)
		var eventName string
		var dataBuilder strings.Builder
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				return nil, err
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" { // frame end
				break
			}
			if strings.HasPrefix(line, ":") {
				// comment/heartbeat, ignore
				continue
			}
			if strings.HasPrefix(line, "event:") {
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
				continue
			}
			if strings.HasPrefix(line, "data:") {
				if dataBuilder.Len() > 0 {
					dataBuilder.WriteByte('\n')
				}
				dataBuilder.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
				continue
			}
			// ignore id: and others for test
		}
		if eventName == "workspace.event" {
			var evt sseWorkspaceEvent
			if err := json.Unmarshal([]byte(dataBuilder.String()), &evt); err == nil {
				return &evt, nil
			}
		}
		// else continue
	}
}

// Tests

func TestHTTP_SSE_Events_OnWriteAndDelete(t *testing.T) {
	bin := buildBinary(t)
	wsRoot, err := os.MkdirTemp("", "mcp-ws-root-events")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(wsRoot) })

	host := "127.0.0.1"
	port := "18090"
	_ = startServer(t, bin, wsRoot, host, port)

	// Create workspace
	createEP := fmt.Sprintf("http://%s:%s/api/tools/workspace_create", host, port)
	resp := restPOST(t, createEP, map[string]any{"name": "Events Test"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var ws struct {
		WorkspaceID string `json:"workspaceId"`
		Path        string `json:"path"`
	}
	mustJSON(t, resp.Body, &ws)
	resp.Body.Close()

	// Subscribe SSE for this workspace
	eventsURL := fmt.Sprintf("http://%s:%s/events?workspaceId=%s", host, port, ws.WorkspaceID)
	stream, rd := openSSE(t, eventsURL)
	defer stream.Body.Close()

	// Write a file -> expect file.created
	writeEP := fmt.Sprintf("http://%s:%s/api/tools/fs_write_file", host, port)
	respW := restPOST(t, writeEP, map[string]any{"workspaceId": ws.WorkspaceID, "path": "a.txt", "content": "hello"})
	require.Equal(t, http.StatusOK, respW.StatusCode)
	respW.Body.Close()

	evt, err := readNextWorkspaceEvent(rd, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "file.created", evt.Type)
	require.Equal(t, "a.txt", evt.Path)
	require.False(t, evt.IsDir)

	// Delete the file -> expect file.deleted
	delEP := fmt.Sprintf("http://%s:%s/api/tools/fs_delete_file", host, port)
	respD := restPOST(t, delEP, map[string]any{"workspaceId": ws.WorkspaceID, "path": "a.txt"})
	require.Equal(t, http.StatusOK, respD.StatusCode)
	respD.Body.Close()

	evt2, err := readNextWorkspaceEvent(rd, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "file.deleted", evt2.Type)
	require.Equal(t, "a.txt", evt2.Path)
	require.False(t, evt2.IsDir)
}

func TestHTTP_REST_NoOpWrite_NoCommit_NoEvent(t *testing.T) {
	bin := buildBinary(t)
	wsRoot, err := os.MkdirTemp("", "mcp-ws-root-noop")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(wsRoot) })

	host := "127.0.0.1"
	port := "18091"
	_ = startServer(t, bin, wsRoot, host, port)

	// Create workspace
	createEP := fmt.Sprintf("http://%s:%s/api/tools/workspace_create", host, port)
	resp := restPOST(t, createEP, map[string]any{"name": "NoOp Test"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var ws struct {
		WorkspaceID string `json:"workspaceId"`
		Path        string `json:"path"`
	}
	mustJSON(t, resp.Body, &ws)
	resp.Body.Close()

	// Subscribe events
	eventsURL := fmt.Sprintf("http://%s:%s/events?workspaceId=%s", host, port, ws.WorkspaceID)
	stream, rd := openSSE(t, eventsURL)
	defer stream.Body.Close()

	writeEP := fmt.Sprintf("http://%s:%s/api/tools/fs_write_file", host, port)

	// 1) Initial write -> expect file.created
	respW1 := restPOST(t, writeEP, map[string]any{"workspaceId": ws.WorkspaceID, "path": "b.txt", "content": "init"})
	require.Equal(t, http.StatusOK, respW1.StatusCode)
	respW1.Body.Close()

	evt1, err := readNextWorkspaceEvent(rd, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "file.created", evt1.Type)

	// 2) No-op write (same content) -> expect no new event within 800ms
	respW2 := restPOST(t, writeEP, map[string]any{"workspaceId": ws.WorkspaceID, "path": "b.txt", "content": "init"})
	require.Equal(t, http.StatusOK, respW2.StatusCode)
	// Verify response body shows BytesWritten:0 (best-effort)
	var wOut struct {
		Path         string `json:"path"`
		BytesWritten int    `json:"bytesWritten"`
		Overwritten  bool   `json:"overwritten"`
		Commit       string `json:"commit"`
	}
	mustJSON(t, respW2.Body, &wOut)
	respW2.Body.Close()
	require.Equal(t, 0, wOut.BytesWritten)
	require.Equal(t, "", wOut.Commit)

	// Ensure no event arrives in short window
	_, err = readNextWorkspaceEvent(rd, 800*time.Millisecond)
	require.Error(t, err, "expected no event for no-op write")
}

func TestHTTP_REST_Preconditions_EtagConflict409(t *testing.T) {
	bin := buildBinary(t)
	wsRoot, err := os.MkdirTemp("", "mcp-ws-root-409")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(wsRoot) })

	host := "127.0.0.1"
	port := "18092"
	_ = startServer(t, bin, wsRoot, host, port)

	// Create workspace
	createEP := fmt.Sprintf("http://%s:%s/api/tools/workspace_create", host, port)
	resp := restPOST(t, createEP, map[string]any{"name": "Precond Test"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var ws struct {
		WorkspaceID string `json:"workspaceId"`
		Path        string `json:"path"`
	}
	mustJSON(t, resp.Body, &ws)
	resp.Body.Close()

	// Write v1
	writeEP := fmt.Sprintf("http://%s:%s/api/tools/fs_write_file", host, port)
	respW1 := restPOST(t, writeEP, map[string]any{"workspaceId": ws.WorkspaceID, "path": "c.txt", "content": "v1"})
	require.Equal(t, http.StatusOK, respW1.StatusCode)
	respW1.Body.Close()

	// Read to get etag and head
	readEP := fmt.Sprintf("http://%s:%s/api/tools/fs_read_text_file", host, port)
	respR := restPOST(t, readEP, map[string]any{"workspaceId": ws.WorkspaceID, "path": "c.txt"})
	require.Equal(t, http.StatusOK, respR.StatusCode)
	var rOut struct {
		Content       string `json:"content"`
		Etag          string `json:"etag"`
		WorkspaceHead string `json:"workspaceHead"`
	}
	mustJSON(t, respR.Body, &rOut)
	respR.Body.Close()
	require.NotEmpty(t, rOut.Etag)

	// Write v2 to change the file and HEAD
	respW2 := restPOST(t, writeEP, map[string]any{"workspaceId": ws.WorkspaceID, "path": "c.txt", "content": "v2"})
	require.Equal(t, http.StatusOK, respW2.StatusCode)
	respW2.Body.Close()

	// Attempt write with stale etag/head -> expect 409
	// Note: REST mirror maps errors starting with "CONFLICT:" to 409
	payload := map[string]any{
		"workspaceId":          ws.WorkspaceID,
		"path":                 "c.txt",
		"content":              "v3",
		"ifMatchFileEtag":      rOut.Etag,
		"ifMatchWorkspaceHead": rOut.WorkspaceHead,
	}
	respW3 := restPOST(t, writeEP, payload)
	defer respW3.Body.Close()
	require.Equal(t, http.StatusConflict, respW3.StatusCode)
}
