package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/validation"
)

func TestCheckWorkspace(t *testing.T) {
	dir := t.TempDir()
	if c := checkWorkspace(dir); c.status != statusOK {
		t.Fatalf("writable dir should be OK, got %s: %s", c.status, c.detail)
	}
	if c := checkWorkspace(filepath.Join(dir, "nope")); c.status != statusFail {
		t.Fatalf("missing dir should FAIL, got %s", c.status)
	}
	if c := checkWorkspace(""); c.status != statusWarn {
		t.Fatalf("empty workspace should WARN, got %s", c.status)
	}
}

func TestCheckProviderAuth(t *testing.T) {
	env := func(k string) string {
		if k == "ANTHROPIC_API_KEY" {
			return "sk-present"
		}
		return ""
	}
	agents := []orchestrator.AgentConfig{
		{Provider: "anthropic"}, // key present
		{Provider: "ollama"},    // no key needed
		{Provider: "claude_code"},
	}
	if c := checkProviderAuth(agents, env); c.status != statusOK {
		t.Fatalf("all creds present should be OK, got %s: %s", c.status, c.detail)
	}

	agents = append(agents, orchestrator.AgentConfig{Provider: "openai"}) // missing key
	c := checkProviderAuth(agents, env)
	if c.status != statusFail || !strings.Contains(c.detail, "OPENAI_API_KEY") {
		t.Fatalf("missing openai key should FAIL naming the env var, got %s: %s", c.status, c.detail)
	}

	if c := checkProviderAuth(nil, env); c.status != statusWarn {
		t.Fatalf("no agents should WARN, got %s", c.status)
	}
}

func TestCheckValidationProfile(t *testing.T) {
	reg := validation.NewRegistry() // has go_test_all
	if c := checkValidationProfile(reg, "go_test_all"); c.status != statusOK {
		t.Fatalf("registered profile should be OK, got %s: %s", c.status, c.detail)
	}
	if c := checkValidationProfile(reg, "nonexistent_profile"); c.status != statusWarn {
		t.Fatalf("unknown profile should WARN, got %s", c.status)
	}
}

func TestCheckGitRemote(t *testing.T) {
	if c := checkGitRemote(".", func(string) bool { return true }); c.status != statusOK {
		t.Fatalf("remote present should be OK, got %s", c.status)
	}
	if c := checkGitRemote(".", func(string) bool { return false }); c.status != statusWarn {
		t.Fatalf("no remote should WARN, got %s", c.status)
	}
}

func TestCheckNATSEmbedded(t *testing.T) {
	if c := checkNATS(""); c.status != statusOK {
		t.Fatalf("empty URL (embedded) should be OK, got %s", c.status)
	}
}

func TestCheckRemembranceDisabled(t *testing.T) {
	c := checkRemembrance(orchestrator.RemembranceConfig{Enabled: false}, nil)
	if c.status != statusOK || !strings.Contains(c.detail, "disabled") {
		t.Fatalf("disabled remembrance should be OK/disabled, got %s: %s", c.status, c.detail)
	}
}

func TestWorstStatusAndRender(t *testing.T) {
	checks := []doctorCheck{
		{name: "a", status: statusOK, detail: "fine"},
		{name: "b", status: statusWarn, detail: "meh"},
	}
	if w, failed := worstStatus(checks); w != statusWarn || failed {
		t.Fatalf("warn+ok should be WARN, not failed; got %s failed=%v", w, failed)
	}
	out := renderDoctor(checks)
	if !strings.Contains(out, "OK with warnings") {
		t.Fatalf("expected warnings result line:\n%s", out)
	}

	checks = append(checks, doctorCheck{name: "c", status: statusFail, detail: "broken"})
	if w, failed := worstStatus(checks); w != statusFail || !failed {
		t.Fatalf("any fail should be FAIL+failed, got %s failed=%v", w, failed)
	}
	if out := renderDoctor(checks); !strings.Contains(out, "FAIL") {
		t.Fatalf("expected FAIL result line:\n%s", out)
	}

	allok := []doctorCheck{{name: "a", status: statusOK}}
	if !strings.Contains(renderDoctor(allok), "all systems go") {
		t.Fatalf("all OK should say all systems go")
	}
}

// Sanity: the probe file is cleaned up.
func TestCheckWorkspaceCleansProbe(t *testing.T) {
	dir := t.TempDir()
	checkWorkspace(dir)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), "doctor-probe") {
			t.Fatalf("probe file left behind: %s", e.Name())
		}
	}
}

func TestDoctorToJSON(t *testing.T) {
	checks := []doctorCheck{
		{name: "workspace", status: statusOK, detail: "ok"},
		{name: "nats", status: statusWarn, detail: "unreachable"},
	}
	data, err := doctorToJSON(checks)
	if err != nil {
		t.Fatalf("doctorToJSON: %v", err)
	}
	var rep map[string]any
	if err := jsonUnmarshal(data, &rep); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if rep["result"] != "warn" {
		t.Fatalf("expected warn result, got %v", rep["result"])
	}
	checks = append(checks, doctorCheck{name: "x", status: statusFail, detail: "broken"})
	data, _ = doctorToJSON(checks)
	_ = jsonUnmarshal(data, &rep)
	if rep["result"] != "fail" {
		t.Fatalf("expected fail result, got %v", rep["result"])
	}
}
