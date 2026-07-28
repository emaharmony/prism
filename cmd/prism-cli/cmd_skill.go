package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/emaharmony/prism/internal/skill"
	"gopkg.in/yaml.v3"
)

// cmdSkill handles the `prism skill` subcommand.
//
// Usage:
//   prism skill install <git-url> [--force]   Install a skill from a git repo
//   prism skill list                          List installed skills
//   prism skill remove <name>                 Remove an installed skill
//   prism skill update <name>                 Update an installed skill
func cmdSkill(args []string) {
	fs := flag.NewFlagSet("skill", flag.ExitOnError)
	force := fs.Bool("force", false, "force update/overwrite existing skill")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: prism skill <install|list|remove|update> [args]")
		os.Exit(1)
	}

	subcommand := fs.Arg(0)
	rest := fs.Args()[1:]
	skillsDir := resolveSkillsDir()

	switch subcommand {
	case "install":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "Usage: prism skill install <git-url> [--force]")
			os.Exit(1)
		}
		s, err := skill.Install(rest[0], skillsDir, *force)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Installed skill: %s\n", s.Name)
		if s.Description != "" {
			fmt.Printf("   %s\n", s.Description)
		}

	case "list":
		infos, err := skill.ListInstalled(skillsDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if len(infos) == 0 {
			fmt.Println("No skills installed.")
			return
		}
		fmt.Printf("Installed skills (%d):\n", len(infos))
		for _, info := range infos {
			desc := info.Description
			if len(desc) > 80 {
				desc = desc[:80] + "..."
			}
			fmt.Printf("  - %s: %s\n", info.Name, desc)
		}

	case "remove":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "Usage: prism skill remove <name>")
			os.Exit(1)
		}
		if err := skill.Uninstall(rest[0], skillsDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Removed skill: %s\n", rest[0])

	case "update":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "Usage: prism skill update <name>")
			os.Exit(1)
		}
		s, err := skill.Update(rest[0], skillsDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Updated skill: %s\n", s.Name)

	default:
		fmt.Fprintf(os.Stderr, "Unknown skill command: %s\n", subcommand)
		os.Exit(1)
	}
}

// resolveSkillsDir determines the skills directory from prism.yaml or defaults.
func resolveSkillsDir() string {
	data, err := os.ReadFile("prism.yaml")
	if err == nil {
		var cfg struct {
			Prism struct {
				Workspace string `yaml:"workspace"`
			} `yaml:"prism"`
		}
		if yaml.Unmarshal(data, &cfg) == nil && cfg.Prism.Workspace != "" {
			return filepath.Join(cfg.Prism.Workspace, "skills")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".prism", "skills")
}
