package transport

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"mcp-workspace-manager/pkg/mcp"
	"os"
)

// ToolHandler is a function that handles an MCP tool request.
// For now, this is a placeholder. In the future, it will dispatch to the correct tool.
type ToolHandler func(request *mcp.Request) *mcp.Response

// RunStdio starts the MCP server over standard I/O.
// It reads JSON requests from stdin, passes them to a handler, and writes JSON responses to stdout.
func RunStdio(handler ToolHandler) {
	slog.Info("Starting stdio transport listener")
	reader := bufio.NewReader(os.Stdin)
	writer := os.Stdout

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				slog.Error("Error reading from stdin", "error", err)
			} else {
				slog.Info("EOF received, shutting down stdio listener.")
			}
			return // Exit on EOF or read error
		}

		var req mcp.Request
		if err := json.Unmarshal(line, &req); err != nil {
			slog.Error("Failed to decode MCP request", "error", err, "raw_request", string(line))
			// Send a response for a malformed request
			resp := &mcp.Response{
				Error: mcp.NewError("INVALID_INPUT", "Failed to parse JSON request", nil),
			}
			sendResponse(writer, resp)
			continue
		}

		slog.Debug("Received MCP request", "tool", req.Tool, "id", string(req.ID))

		// Pass the request to the handler and get a response
		resp := handler(&req)

		// Send the response
		if err := sendResponse(writer, resp); err != nil {
			slog.Error("Failed to write MCP response", "error", err)
		}
	}
}

// sendResponse marshals and writes a response to the given writer.
func sendResponse(writer io.Writer, resp *mcp.Response) error {
	slog.Debug("Sending MCP response", "id", string(resp.ID))
	respBytes, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	_, err = writer.Write(respBytes)
	if err != nil {
		return err
	}
	_, err = writer.Write([]byte("\n")) // MCP messages are newline-delimited
	return err
}