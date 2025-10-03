package main

import (
	"flag"
	"fmt"
	"log/slog"
	"mcp-workspace-manager/pkg/mcp"
	"mcp-workspace-manager/pkg/tool"
	"mcp-workspace-manager/pkg/transport"
	"mcp-workspace-manager/pkg/workspace"
	"os"
	"strings"
)

// Config holds the application configuration.
type Config struct {
	WorkspacesRoot string
	Transport      string
	Port           int
	LogFormat      string
	LogLevel       slog.Level
}

func main() {
	cfg := &Config{}
	flag.StringVar(&cfg.WorkspacesRoot, "workspaces-root", os.Getenv("WORKSPACES_ROOT"), "Parent directory for all workspaces (env: WORKSPACES_ROOT)")
	flag.StringVar(&cfg.Transport, "transport", os.Getenv("MCP_TRANSPORT"), "Transport to use: 'stdio' or 'http' (env: MCP_TRANSPORT)")
	flag.IntVar(&cfg.Port, "port", 8080, "Port for HTTP transport (env: PORT)")
	flag.StringVar(&cfg.LogFormat, "log-format", "text", "Log format: 'text' or 'json'")
	flag.String("log-level", "info", "Log level: 'debug', 'info', 'warn', 'error'")
	flag.Parse()

	if err := validateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	setupLogger(cfg)

	slog.Info("Starting MCP Workspace Manager",
		"version", "0.1.0",
		"transport", cfg.Transport,
		"workspaces-root", cfg.WorkspacesRoot,
	)

	// --- Initialize Managers and Services ---
	workspaceManager, err := workspace.NewManager(cfg.WorkspacesRoot)
	if err != nil {
		slog.Error("Failed to initialize workspace manager", "error", err)
		os.Exit(1)
	}

	// --- Setup Tool Registry and Dispatcher ---
	toolRegistry := tool.NewRegistry()
	tool.RegisterWorkspaceTools(toolRegistry, workspaceManager)
	tool.RegisterFSTools(toolRegistry, workspaceManager)

	// The main handler now uses the registry to dispatch requests.
	toolHandler := func(req *mcp.Request) *mcp.Response {
		return toolRegistry.Dispatch(req)
	}

	// --- Start Transport Listener ---
	if cfg.Transport == "http" {
		transport.RunHTTP(cfg.Port, toolHandler)
	} else {
		transport.RunStdio(toolHandler)
	}
}

func validateConfig(cfg *Config) error {
	if cfg.WorkspacesRoot == "" {
		return fmt.Errorf("--workspaces-root is required")
	}
	if cfg.Transport == "" {
		return fmt.Errorf("--transport is required")
	}
	if cfg.Transport != "stdio" && cfg.Transport != "http" {
		return fmt.Errorf("--transport must be 'stdio' or 'http'")
	}
	return nil
}

func setupLogger(cfg *Config) {
	logLevelFlag := flag.Lookup("log-level").Value.String()
	logLevelMap := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	level, exists := logLevelMap[strings.ToLower(logLevelFlag)]
	if !exists {
		level = slog.LevelInfo
	}
	cfg.LogLevel = level

	var logHandler slog.Handler
	if cfg.LogFormat == "json" {
		logHandler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})
	} else {
		logHandler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})
	}
	slog.SetDefault(slog.New(logHandler))
}