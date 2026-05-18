package main

import (
	"fmt"
	"os"
	"github.com/emaharmony/prism/internal/dashboard"
)

func executeDashboard(port, runDir, policyDir string) {
	server := dashboard.NewServer(":"+port, runDir, policyDir)
	if err := server.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
