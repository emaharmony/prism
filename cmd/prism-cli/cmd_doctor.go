// Package main implements the `prism doctor` subcommand: a preflight check that
// surfaces the silent-failure modes that otherwise only show up mid-run.
//
// Usage:
//
//	prism doctor [--config prism.yaml]
//
// It verifies the things an autonomous run depends on — workspace writability,
// provider credentials, a registered validation profile (so the verification gate
// can run), a git remote (so commits can push), NATS, and Remembrance — and prints
// a clear OK / WARN / FAIL report. Exit code is non-zero if any check FAILs, so it
// is usable in CI / startup scripts.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/validation"
	"github.com/nats-io/nats.go"
)

type checkStatus string

const (
	statusOK   checkStatus = "OK"
	statusWarn checkStatus = "WARN"
	statusFail checkStatus = "FAIL"
)

// doctorCheck is the result of a single preflight check.
type doctorCheck struct {
	name   string
	status checkStatus
	detail string
}

// providerKeyEnv maps an LLM provider to the env var holding its API key. A
// provider absent from the map needs no key (local or subscription-based).
var providerKeyEnv = map[string]string{
	"openai":    "OPENAI_API_KEY",
	"anthropic": "ANTHROPIC_API_KEY",
	"gemini":    "GEMINI_API_KEY",
}

// checkWorkspace verifies the workspace root exists and is writable.
func checkWorkspace(path string) doctorCheck {
	c := doctorCheck{name: "workspace"}
	if path == "" {
		c.status, c.detail = statusWarn, "no workspace configured (context injection limited)"
		return c
	}
	info, err := os.Stat(path)
	if err != nil {
		c.status, c.detail = statusFail, fmt.Sprintf("%s: %v", path, err)
		return c
	}
	if !info.IsDir() {
		c.status, c.detail = statusFail, fmt.Sprintf("%s is not a directory", path)
		return c
	}
	probe := path + string(os.PathSeparator) + ".prism-doctor-probe"
	if werr := os.WriteFile(probe, []byte("x"), 0o600); werr != nil {
		c.status, c.detail = statusWarn, fmt.Sprintf("%s not writable: %v", path, werr)
		return c
	}
	_ = os.Remove(probe)
	c.status, c.detail = statusOK, path
	return c
}

// checkProviderAuth verifies each configured agent's provider has the credential
// it needs. getenv is injected for testability.
func checkProviderAuth(agents []orchestrator.AgentConfig, getenv func(string) string) doctorCheck {
	c := doctorCheck{name: "provider auth"}
	if len(agents) == 0 {
		c.status, c.detail = statusWarn, "no agents configured"
		return c
	}
	var missing []string
	checked := map[string]bool{}
	for _, a := range agents {
		env, needs := providerKeyEnv[strings.ToLower(a.Provider)]
		if !needs || checked[a.Provider] {
			continue
		}
		checked[a.Provider] = true
		if getenv(env) == "" {
			missing = append(missing, fmt.Sprintf("%s ($%s)", a.Provider, env))
		}
	}
	if len(missing) > 0 {
		c.status, c.detail = statusFail, "missing credentials for: "+strings.Join(missing, ", ")
		return c
	}
	c.status, c.detail = statusOK, "all configured providers have credentials"
	return c
}

// checkValidationProfile verifies a verification profile is registered, so the
// EXECUTION verification gate has something to run.
func checkValidationProfile(reg *validation.Registry, profile string) doctorCheck {
	c := doctorCheck{name: "validation profile"}
	if profile == "" {
		profile = "go_test_all"
	}
	if _, err := reg.Resolve(profile); err != nil {
		c.status, c.detail = statusWarn, fmt.Sprintf("profile %q not registered; verification gate will be skipped", profile)
		return c
	}
	c.status, c.detail = statusOK, profile+" registered"
	return c
}

// checkGitRemote reports whether the repo has a remote (so commits can push).
// remoteFn is injected for testability (defaults to repoHasRemote).
func checkGitRemote(repoPath string, remoteFn func(string) bool) doctorCheck {
	c := doctorCheck{name: "git remote"}
	if remoteFn(repoPath) {
		c.status, c.detail = statusOK, "remote configured (push enabled)"
	} else {
		c.status, c.detail = statusWarn, "no git remote; loop runs with push disabled"
	}
	return c
}

// checkNATS reports bus reachability. An empty URL is the embedded-NATS case
// (started by `prism serve`), which is fine.
func checkNATS(url string) doctorCheck {
	c := doctorCheck{name: "nats"}
	if url == "" {
		c.status, c.detail = statusOK, "embedded (started by `prism serve`)"
		return c
	}
	conn, err := nats.Connect(url, nats.Timeout(2*time.Second), nats.MaxReconnects(0))
	if err != nil {
		c.status, c.detail = statusWarn, fmt.Sprintf("%s unreachable: %v", url, err)
		return c
	}
	conn.Close()
	c.status, c.detail = statusOK, url
	return c
}

// checkRemembrance reports memory-service reachability when enabled.
func checkRemembrance(cfg orchestrator.RemembranceConfig, get func(string) (*http.Response, error)) doctorCheck {
	c := doctorCheck{name: "remembrance"}
	if !cfg.Enabled {
		c.status, c.detail = statusOK, "disabled"
		return c
	}
	if cfg.URL == "" {
		c.status, c.detail = statusWarn, "enabled but no url configured"
		return c
	}
	resp, err := get(strings.TrimRight(cfg.URL, "/") + "/health")
	if err != nil {
		c.status, c.detail = statusWarn, fmt.Sprintf("%s unreachable: %v", cfg.URL, err)
		return c
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		c.status, c.detail = statusWarn, fmt.Sprintf("%s returned %s", cfg.URL, resp.Status)
		return c
	}
	c.status, c.detail = statusOK, cfg.URL
	return c
}

// worstStatus returns the most severe status across checks and whether any failed.
func worstStatus(checks []doctorCheck) (checkStatus, bool) {
	worst := statusOK
	failed := false
	for _, c := range checks {
		switch c.status {
		case statusFail:
			worst, failed = statusFail, true
		case statusWarn:
			if worst != statusFail {
				worst = statusWarn
			}
		}
	}
	return worst, failed
}

func statusGlyph(s checkStatus) string {
	switch s {
	case statusOK:
		return "✅"
	case statusWarn:
		return "⚠️ "
	default:
		return "❌"
	}
}

// doctorCheckJSON / doctorReportJSON are the machine-readable view for `--json`.
type doctorCheckJSON struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type doctorReportJSON struct {
	Checks []doctorCheckJSON `json:"checks"`
	Result string            `json:"result"` // ok | warn | fail
}

// doctorToJSON marshals the checks plus an overall result (pure, testable).
func doctorToJSON(checks []doctorCheck) ([]byte, error) {
	rep := doctorReportJSON{Checks: make([]doctorCheckJSON, 0, len(checks))}
	for _, c := range checks {
		rep.Checks = append(rep.Checks, doctorCheckJSON{Name: c.name, Status: string(c.status), Detail: c.detail})
	}
	worst, failed := worstStatus(checks)
	switch {
	case failed:
		rep.Result = "fail"
	case worst == statusWarn:
		rep.Result = "warn"
	default:
		rep.Result = "ok"
	}
	return json.MarshalIndent(rep, "", "  ")
}

// renderDoctor formats the check report.
func renderDoctor(checks []doctorCheck) string {
	var b strings.Builder
	b.WriteString("🩺 prism doctor\n")
	b.WriteString(strings.Repeat("─", 56) + "\n")
	for _, c := range checks {
		fmt.Fprintf(&b, "  %s %-20s %s\n", statusGlyph(c.status), c.name, c.detail)
	}
	worst, failed := worstStatus(checks)
	b.WriteString(strings.Repeat("─", 56) + "\n")
	switch {
	case failed:
		b.WriteString("Result: FAIL — fix the ❌ checks before running.\n")
	case worst == statusWarn:
		b.WriteString("Result: OK with warnings — the loop will run but some features are limited.\n")
	default:
		b.WriteString("Result: all systems go.\n")
	}
	return b.String()
}

// executeDoctor is the `prism doctor` entry point.
func executeDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	configPath := fs.String("config", "prism.yaml", "Path to prism.yaml configuration file")
	asJSON := fs.Bool("json", false, "Emit the report as JSON")
	fs.Parse(args)

	cfg, err := orchestrator.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ config: %v\n", err)
		os.Exit(1)
	}

	repoPath := cfg.Prism.Workspace
	if repoPath == "" {
		repoPath = "."
	}

	checks := []doctorCheck{
		checkWorkspace(cfg.Prism.Workspace),
		checkProviderAuth(cfg.Agents, os.Getenv),
		checkValidationProfile(validation.NewRegistry(), ""),
		checkGitRemote(repoPath, repoHasRemote),
		checkNATS(cfg.Prism.NATSURL),
		checkRemembrance(cfg.Remembrance, func(u string) (*http.Response, error) {
			client := &http.Client{Timeout: 2 * time.Second}
			return client.Get(u)
		}),
	}

	if *asJSON {
		data, mErr := doctorToJSON(checks)
		if mErr != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", mErr)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		fmt.Print(renderDoctor(checks))
	}
	if _, failed := worstStatus(checks); failed {
		os.Exit(1)
	}
}
