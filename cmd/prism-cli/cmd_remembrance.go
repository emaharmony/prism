// Package main implements the `prism remembrance` subcommand.
//
// Usage:
//
//	prism remembrance health [--config prism.yaml] [--url <url>]
//	prism remembrance status [--config prism.yaml] [--url <url>]
//	prism remembrance serve  [--config prism.yaml] [--dir <path>] [--python <exe>]
//	                         [--module rememberance_mcp.serve] [--nats <url>]
//
// `health` and `status` probe the configured Remembrance service over HTTP.
// `serve` launches the external Remembrance (Python) service in the foreground,
// deriving host/port from the configured Remembrance URL. It is a convenience
// wrapper around `python -m <module> --host <host> --port <port>`; Prism does
// not embed Remembrance, it only starts the separate process.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/remembrance"
)

// executeRemembrance dispatches `prism remembrance <subcommand>`.
func executeRemembrance(args []string) {
	if len(args) < 1 {
		printRemembranceUsage()
		os.Exit(1)
	}
	switch args[0] {
	case "health":
		remembranceHealth(args[1:])
	case "status":
		remembranceStatus(args[1:])
	case "serve":
		remembranceServe(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown remembrance subcommand %q\n", args[0])
		printRemembranceUsage()
		os.Exit(1)
	}
}

func printRemembranceUsage() {
	fmt.Println("Usage:")
	fmt.Println("  prism remembrance health [--config prism.yaml] [--url <url>]")
	fmt.Println("  prism remembrance status [--config prism.yaml] [--url <url>]")
	fmt.Println("  prism remembrance serve  [--config prism.yaml] [--dir <path>] [--python <exe>]")
	fmt.Println("                           [--module rememberance_mcp.serve] [--nats <url>]")
}

// remembranceBaseURL resolves the Remembrance base URL from --url, then config,
// then the default of http://localhost:18790.
func remembranceBaseURL(configPath, override string) string {
	if override != "" {
		return strings.TrimSuffix(override, "/")
	}
	if cfg, err := orchestrator.LoadConfig(configPath); err == nil && cfg.Remembrance.URL != "" {
		return strings.TrimSuffix(cfg.Remembrance.URL, "/")
	}
	return "http://localhost:18790"
}

func remembranceHealth(args []string) {
	fs := flag.NewFlagSet("remembrance health", flag.ExitOnError)
	configPath := fs.String("config", "prism.yaml", "Path to prism.yaml configuration file")
	urlFlag := fs.String("url", "", "Remembrance URL (overrides config)")
	fs.Parse(args)

	base := remembranceBaseURL(*configPath, *urlFlag)
	if remembrance.NewClient(base).IsAvailable() {
		fmt.Printf("✅ Remembrance healthy at %s\n", base)
		return
	}
	fmt.Printf("❌ Remembrance not reachable at %s/v1/health\n", base)
	fmt.Println("   Start it with: prism remembrance serve")
	os.Exit(1)
}

func remembranceStatus(args []string) {
	fs := flag.NewFlagSet("remembrance status", flag.ExitOnError)
	configPath := fs.String("config", "prism.yaml", "Path to prism.yaml configuration file")
	urlFlag := fs.String("url", "", "Remembrance URL (overrides config)")
	fs.Parse(args)

	base := remembranceBaseURL(*configPath, *urlFlag)
	fmt.Printf("🧠 Remembrance @ %s\n\n", base)

	if !remembrance.NewClient(base).IsAvailable() {
		fmt.Println("  health: unreachable")
		os.Exit(1)
	}
	fmt.Println("  health: ok")

	// Stats are best-effort: the endpoint exists on rememberance-mcp but not on
	// every Remembrance implementation, so a failure here is not fatal.
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Get(base + "/stats")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	var stats map[string]any
	if json.Unmarshal(body, &stats) != nil {
		return
	}
	pretty, _ := json.MarshalIndent(stats, "  ", "  ")
	fmt.Printf("  stats:\n  %s\n", pretty)
}

func remembranceServe(args []string) {
	fs := flag.NewFlagSet("remembrance serve", flag.ExitOnError)
	configPath := fs.String("config", "prism.yaml", "Path to prism.yaml configuration file")
	urlFlag := fs.String("url", "", "Remembrance URL (host/port derived from this)")
	dir := fs.String("dir", "", "Working directory of the Remembrance service")
	python := fs.String("python", "python", "Python interpreter to use (e.g. a venv python.exe)")
	module := fs.String("module", "rememberance_mcp.serve", "Python module to run")
	nats := fs.String("nats", "", "NATS URL for event-driven auto-capture (empty = --no-nats)")
	fs.Parse(args)

	base := remembranceBaseURL(*configPath, *urlFlag)
	host, port := hostPortFromURL(base)

	cmdArgs := []string{"-m", *module, "--host", host, "--port", port}
	if *nats != "" {
		cmdArgs = append(cmdArgs, "--nats", *nats)
	} else {
		cmdArgs = append(cmdArgs, "--no-nats")
	}

	cmd := exec.Command(*python, cmdArgs...)
	if *dir != "" {
		cmd.Dir = *dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	fmt.Printf("🧠 Starting Remembrance: %s %s\n", *python, strings.Join(cmdArgs, " "))
	if *dir != "" {
		fmt.Printf("   cwd: %s\n", *dir)
	}
	fmt.Printf("   serving %s — press Ctrl+C to stop\n\n", base)

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting Remembrance: %v\n", err)
		fmt.Fprintln(os.Stderr, "Hint: pass --python <venv python> and --dir <remembrance repo> if the module is not on PATH.")
		os.Exit(1)
	}

	// Backstop: kill the child if Prism is interrupted. In a shared console,
	// Ctrl+C is also delivered to the child directly, so it usually exits first.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	if err := cmd.Wait(); err != nil {
		os.Exit(1)
	}
}

// hostPortFromURL extracts host and port from a Remembrance URL, falling back
// to 127.0.0.1:18790 for any missing piece.
func hostPortFromURL(raw string) (string, string) {
	host, port := "127.0.0.1", "18790"
	if u, err := url.Parse(raw); err == nil {
		if h := u.Hostname(); h != "" {
			host = h
		}
		if p := u.Port(); p != "" {
			port = p
		}
	}
	return host, port
}
