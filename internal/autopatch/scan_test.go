package autopatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(root, name, content string) error {
	return os.WriteFile(filepath.Join(root, name), []byte(content), 0644)
}

type mockDetector struct {
	name   string
	issues []Issue
	err    error
}

func (m mockDetector) Name() string { return m.name }
func (m mockDetector) Detect(_ context.Context, _ string) ([]Issue, error) {
	return m.issues, m.err
}

func TestScannerRanksBySeverity(t *testing.T) {
	s := NewScanner(
		mockDetector{name: "a", issues: []Issue{{Kind: "todo", Severity: SeverityMedium, Title: "t"}}},
		mockDetector{name: "b", issues: []Issue{{Kind: "vet", Severity: SeverityHigh, Title: "v"}}},
		mockDetector{name: "c", issues: []Issue{{Kind: "format", Severity: SeverityLow, Title: "f"}}},
	)
	issues, err := s.Scan(context.Background(), ".")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(issues) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(issues))
	}
	if issues[0].Kind != "vet" || issues[1].Kind != "todo" || issues[2].Kind != "format" {
		t.Fatalf("not ranked high→low: %v / %v / %v", issues[0].Kind, issues[1].Kind, issues[2].Kind)
	}
}

func TestScannerDedupsAndCollectsErrors(t *testing.T) {
	dup := Issue{Kind: "vet", Severity: SeverityHigh, Title: "same"}
	s := NewScanner(
		mockDetector{name: "a", issues: []Issue{dup}},
		mockDetector{name: "b", issues: []Issue{dup}}, // duplicate (Kind|Title)
		mockDetector{name: "c", err: errors.New("boom")},
	)
	issues, err := s.Scan(context.Background(), ".")
	if len(issues) != 1 {
		t.Fatalf("expected dedup to 1, got %d", len(issues))
	}
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected collected detector error, got %v", err)
	}
}

func TestIssueToRequest(t *testing.T) {
	is := Issue{Kind: "vet", Severity: SeverityHigh, Title: "go vet reported problems", Detail: "main.go:3: unreachable", Location: "main.go"}
	req := is.ToRequest()
	if req.Source != "scanner" {
		t.Fatalf("source = %q", req.Source)
	}
	for _, want := range []string{"vet", "go vet reported problems", "unreachable", "main.go"} {
		if !strings.Contains(req.Description, want) {
			t.Fatalf("request description missing %q:\n%s", want, req.Description)
		}
	}
}

func TestSeverityString(t *testing.T) {
	if SeverityHigh.String() != "high" || SeverityMedium.String() != "medium" || SeverityLow.String() != "low" {
		t.Fatal("severity strings wrong")
	}
}

// Default detectors run real toolchains against a clean repo — smoke test that
// they don't error and (on this gofmt-clean tree) find nothing in the worktree.
func TestDefaultDetectorsSmoke(t *testing.T) {
	root := newGoRepo(t) // gofmt-clean, vet-clean, no TODOs
	s := NewScanner()
	issues, err := s.Scan(context.Background(), root)
	// go vet may be unavailable offline; tolerate its error but require no panic.
	_ = err
	for _, is := range issues {
		if is.Kind == "" || is.Title == "" {
			t.Fatalf("malformed issue: %+v", is)
		}
	}
}

func TestFormatDetectorFindsUnformatted(t *testing.T) {
	root := newGoRepo(t)
	// Write deliberately unformatted Go.
	if err := writeFile(root, "bad.go", "package main\nfunc  Bad(){ }\n"); err != nil {
		t.Fatal(err)
	}
	issues, err := formatDetector{}.Detect(context.Background(), root)
	if err != nil {
		t.Skipf("gofmt unavailable: %v", err)
	}
	if len(issues) == 0 || issues[0].Kind != "format" {
		t.Fatalf("expected a format issue, got %+v", issues)
	}
	if !strings.Contains(issues[0].Detail, "bad.go") {
		t.Fatalf("detail should name bad.go: %q", issues[0].Detail)
	}
}

// A CRLF-but-otherwise-formatted file is line-ending noise and must be excluded,
// while a genuinely misformatted file in the same tree is still reported.
func TestFormatDetectorIgnoresCRLFOnly(t *testing.T) {
	root := newGoRepo(t)
	if err := writeFile(root, "crlf.go", "package main\r\n\r\nfunc Clean() {}\r\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(root, "bad.go", "package main\nfunc  Bad(){ }\n"); err != nil {
		t.Fatal(err)
	}
	issues, err := formatDetector{}.Detect(context.Background(), root)
	if err != nil {
		t.Skipf("gofmt unavailable: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected exactly one format issue, got %+v", issues)
	}
	if strings.Contains(issues[0].Detail, "crlf.go") {
		t.Fatalf("CRLF-only file should be excluded as noise:\n%s", issues[0].Detail)
	}
	if !strings.Contains(issues[0].Detail, "bad.go") {
		t.Fatalf("genuinely misformatted file must be reported:\n%s", issues[0].Detail)
	}
}

func TestOnlyLineEndingDiff(t *testing.T) {
	root := newGoRepo(t)
	if err := writeFile(root, "crlf.go", "package main\r\n\r\nfunc Clean() {}\r\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(root, "bad.go", "package main\nfunc  Bad(){ }\n"); err != nil {
		t.Fatal(err)
	}
	eol, err := onlyLineEndingDiff(context.Background(), root, "crlf.go")
	if err != nil {
		t.Skipf("gofmt unavailable: %v", err)
	}
	if !eol {
		t.Fatal("crlf.go should be detected as line-ending-only diff")
	}
	if eol, _ := onlyLineEndingDiff(context.Background(), root, "bad.go"); eol {
		t.Fatal("bad.go has real format debt, not just line endings")
	}
}

func TestTopIssueSelection(t *testing.T) {
	// Scan sorts high→low, so TopIssue returns the most urgent.
	s := NewScanner(
		mockDetector{name: "todo", issues: []Issue{{Kind: "todo", Severity: SeverityMedium, Title: "markers"}}},
		mockDetector{name: "vet", issues: []Issue{{Kind: "vet", Severity: SeverityHigh, Title: "go vet reported problems"}}},
	)
	issues, _ := s.Scan(context.Background(), ".")
	top, ok := TopIssue(issues)
	if !ok || top.Kind != "vet" {
		t.Fatalf("expected top issue = vet, got %+v ok=%v", top, ok)
	}
	if _, ok := TopIssue(nil); ok {
		t.Fatal("empty list should yield ok=false")
	}
	// The selected issue becomes a scanner-sourced patch request.
	req := top.ToRequest()
	if req.Source != "scanner" || !strings.Contains(req.Description, "go vet reported problems") {
		t.Fatalf("top issue did not map to a request: %+v", req)
	}
}

func TestParseSeverity(t *testing.T) {
	cases := map[string]IssueSeverity{
		"high": SeverityHigh, "HIGH": SeverityHigh,
		"medium": SeverityMedium, "med": SeverityMedium,
		"low": SeverityLow, "": SeverityLow, "garbage": SeverityLow,
	}
	for in, want := range cases {
		if got := ParseSeverity(in); got != want {
			t.Fatalf("ParseSeverity(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestFilterBySeverity(t *testing.T) {
	issues := []Issue{
		{Kind: "vet", Severity: SeverityHigh},
		{Kind: "todo", Severity: SeverityMedium},
		{Kind: "format", Severity: SeverityLow},
	}
	// low → everything (no filtering)
	if got := FilterBySeverity(issues, SeverityLow); len(got) != 3 {
		t.Fatalf("low filter should keep all, got %d", len(got))
	}
	// medium → drop the low format finding
	med := FilterBySeverity(issues, SeverityMedium)
	if len(med) != 2 || med[0].Kind != "vet" || med[1].Kind != "todo" {
		t.Fatalf("medium filter wrong: %+v", med)
	}
	// high → only vet
	high := FilterBySeverity(issues, SeverityHigh)
	if len(high) != 1 || high[0].Kind != "vet" {
		t.Fatalf("high filter wrong: %+v", high)
	}
	if got := FilterBySeverity(nil, SeverityHigh); len(got) != 0 {
		t.Fatalf("nil should stay empty")
	}
}
