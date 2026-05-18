// Package dashboard provides a local web dashboard for Prism.
//
// Start with: prism dashboard
// Opens at: http://localhost:8080
//
// The dashboard reads local run data (events, summaries, projections,
// policies) and serves it through a simple HTTP API. The UI is a
// self-contained HTML page with embedded CSS and JavaScript.
//
// Why no framework? Prism is a Go tool. The dashboard should feel like
// part of that tool — self-contained, no build step, no npm, no Webpack.
// One HTML file with inline CSS and JS, embedded in the Go binary.
// This means simplicity and zero deployment complexity, at the cost of
// not having React's component model or Tailwind's utility classes.
// For a local developer tool, that's the right tradeoff.
//
// The dashboard is read-only. It shows data but does NOT approve, deny,
// execute, or mutate anything. All write operations go through the CLI.
package dashboard

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
)

//go:embed static
var staticFS embed.FS

// Server serves the Prism dashboard over HTTP.
//
// The server binds to localhost and serves:
//   - GET /          → Dashboard HTML (self-contained SPA)
//   - GET /api/*     → JSON API for run data
//
// All data comes from local files in the runs/ and policies/ directories.
// No database, no caching, no external services.
type Server struct {
	addr      string // listen address (e.g., ":8080")
	runDir    string // path to runs/ directory
	policyDir string // path to policies/ directory
	mux       *http.ServeMux
}

// NewServer creates a dashboard server that listens on the given address.
//
// Parameters:
//   - addr: listen address (e.g., ":8080")
//   - runDir: path to the runs/ directory containing run data
//   - policyDir: path to the policies/ directory containing policy YAML
func NewServer(addr, runDir, policyDir string) *Server {
	s := &Server{
		addr:      addr,
		runDir:    runDir,
		policyDir: policyDir,
		mux:       http.NewServeMux(),
	}

	// Serve the embedded dashboard UI at /
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("dashboard: failed to create sub filesystem: %v", err)
	}
	s.mux.Handle("/", http.FileServer(http.FS(subFS)))

	// Register API handlers
	s.mux.HandleFunc("/api/runs", s.handleRuns)
	s.mux.HandleFunc("/api/runs/", s.handleRunDetail)
	s.mux.HandleFunc("/api/events/", s.handleEvents)
	s.mux.HandleFunc("/api/projections/", s.handleProjections)
	s.mux.HandleFunc("/api/policies", s.handlePolicies)
	s.mux.HandleFunc("/api/adapters", s.handleAdapters)

	return s
}

// ListenAndServe starts the HTTP server.
// This blocks until the server is stopped.
func (s *Server) ListenAndServe() error {
	addr := s.addr
	if !strings.HasPrefix(addr, ":") {
		addr = ":" + addr
	}

	fmt.Printf("Prism Dashboard: http://localhost%s\n", addr)
	fmt.Println("Press Ctrl+C to stop")
	return http.ListenAndServe(addr, s.mux)
}

// Handler returns the HTTP handler for testing purposes.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// runIDFromPath extracts a run ID from a URL path.
// For "/api/runs/run_01KRC7...", it returns "run_01KRC7...".
func runIDFromPath(path, prefix string) string {
	// Remove the prefix and any trailing slash
	remainder := strings.TrimPrefix(path, prefix)
	remainder = strings.TrimPrefix(remainder, "/")

	// Split on / to get the first segment (the run ID)
	parts := strings.SplitN(remainder, "/", 2)
	return parts[0]
}

// sanitizePath has been replaced by safety.ResolveAndContain (internal/safety).
// Path containment is now centralized in one auditable location.