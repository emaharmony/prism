// Package main — the `prism panel` subcommand: a thin launcher for the desktop pet
// panel (cmd/prism-panel).
//
// The panel is a separate binary (and its own Go module) because its Fyne GUI needs
// cgo + a C compiler, which the pure-Go `prism` CLI deliberately avoids. This
// launcher just locates the co-located `prism-panel` binary and runs it, forwarding
// all arguments (e.g. `prism panel --port 8322`). If it isn't built yet, it prints
// how to build it rather than failing cryptically.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// executePanel finds and launches the prism-panel binary, forwarding args + stdio.
func executePanel(args []string) {
	path, err := findPanelBinary()
	if err != nil {
		fmt.Fprintln(os.Stderr, "prism panel: the desktop pet panel isn't built yet.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "It's a separate binary (needs a C compiler for its GUI). Build it with:")
		fmt.Fprintln(os.Stderr, "    make build-panel")
		fmt.Fprintln(os.Stderr, "or:")
		fmt.Fprintln(os.Stderr, "    cd cmd/prism-panel && CGO_ENABLED=1 go build -o ../../prism-panel .")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "See cmd/prism-panel/README.md for details.")
		os.Exit(1)
	}

	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "prism panel: %v\n", err)
		os.Exit(1)
	}
}

// findPanelBinary locates the prism-panel executable: first next to the running
// prism binary, then in the working directory, then on PATH.
func findPanelBinary() (string, error) {
	name := "prism-panel"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	var candidates []string
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), name))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, name))
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%s not found", name)
}
