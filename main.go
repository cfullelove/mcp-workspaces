package main

import (
	"flag"
	"fmt"
	"log/slog"
	"mcp-workspace-manager/pkg/mcpsdk"
	"mcp-workspace-manager/pkg/workspace"
	"os"
	"strconv"
	"strings"
)

// Config holds the application configuration.
type Config struct {
	WorkspacesRoot string
	Transport      string
	Host           string
	Port           int
	LogFormat      string
	LogLevel       slog.Level
	AuthTokens     []string
}

func main() {
	cfg := &Config{}

	defaultHost := os.Getenv("HOST")
	if defaultHost == "" {
		defaultHost = "127.0.0.1"
	}

	defaultPort := 8080
	if envPort := os.Getenv("PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			defaultPort = p
		} else {
			fmt.Fprintf(os.Stderr, "Invalid PORT value %q, falling back to %d\n", envPort, defaultPort)
		}
	}

	flag.StringVar(&cfg.WorkspacesRoot, "workspaces-root", os.Getenv("WORKSPACES_ROOT"), "Parent directory for all workspaces (env: WORKSPACES_ROOT)")
	flag.StringVar(&cfg.Transport, "transport", os.Getenv("MCP_TRANSPORT"), "Transport to use: 'stdio' or 'http' (env: MCP_TRANSPORT)")
	flag.StringVar(&cfg.Host, "host", defaultHost, "Host/IP to bind for HTTP transport (env: HOST)")
	flag.IntVar(&cfg.Port, "port", defaultPort, "Port for HTTP transport (env: PORT)")
	flag.StringVar(&cfg.LogFormat, "log-format", "text", "Log format: 'text' or 'json'")
	flag.String("log-level", "info", "Log level: 'debug', 'info', 'warn', 'error'")

	var authTokensCSV string
	var authTokenSingle string
	flag.StringVar(&authTokensCSV, "auth-tokens", os.Getenv("AUTH_BEARER_TOKENS"), "Comma-separated list of Bearer tokens for HTTP auth (env: AUTH_BEARER_TOKENS)")
	flag.StringVar(&authTokenSingle, "auth-token", os.Getenv("AUTH_BEARER_TOKEN"), "Single Bearer token for HTTP auth (env: AUTH_BEARER_TOKEN)")

	flag.Parse()

	if err := validateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	setupLogger(cfg)

	cfg.AuthTokens = collectAuthTokens(authTokensCSV, authTokenSingle)

	slog.Info("Starting MCP Workspace Manager",
		"version", "0.1.0",
		"transport", cfg.Transport,
		"host", cfg.Host,
		"port", cfg.Port,
		"workspaces-root", cfg.WorkspacesRoot,
		"auth_enabled", len(cfg.AuthTokens) > 0,
		"auth_tokens", len(cfg.AuthTokens),
	)

	// --- Initialize Managers and Services ---
	workspaceManager, err := workspace.NewManager(cfg.WorkspacesRoot)
	if err != nil {
		slog.Error("Failed to initialize workspace manager", "error", err)
		os.Exit(1)
	}

	// Using MCP SDK server; tool registration happens inside mcpsdk.buildServer.

	// --- Start Transport Listener (MCP SDK) ---
	if cfg.Transport == "http" {
		mcpsdk.RunHTTP(cfg.Host, cfg.Port, workspaceManager, cfg.AuthTokens)
	} else {
		mcpsdk.RunStdio(workspaceManager)
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
	if cfg.Transport == "http" {
		if cfg.Host == "" {
			return fmt.Errorf("--host is required for HTTP transport")
		}
		if cfg.Port <= 0 || cfg.Port > 65535 {
			return fmt.Errorf("--port must be between 1 and 65535")
		}
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

func collectAuthTokens(csv string, single string) []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(s string) {
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, part := range strings.Split(csv, ",") {
		p := strings.TrimSpace(part)
		if p != "" {
			add(p)
		}
	}
	if strings.TrimSpace(single) != "" {
		add(strings.TrimSpace(single))
	}
	return out
}
