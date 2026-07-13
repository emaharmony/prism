package policy

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// PolicyFile represents a YAML policy file with multiple rules.
type PolicyFile struct {
	Policies []PolicyRule `json:"policies" yaml:"policies"`
}

// LoadFromYAML loads policy rules from a YAML byte slice.
func LoadFromYAML(data []byte) ([]PolicyRule, error) {
	var pf PolicyFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("invalid policy YAML: %w", err)
	}

	for _, rule := range pf.Policies {
		if rule.ID == "" {
			return nil, fmt.Errorf("policy rule missing id")
		}
		if rule.Decision == "" {
			return nil, fmt.Errorf("policy rule %q missing decision", rule.ID)
		}
		if rule.Reason == "" {
			return nil, fmt.Errorf("policy rule %q missing reason", rule.ID)
		}
	}

	return pf.Policies, nil
}

// LoadFromDir loads all YAML policy files from a directory into the registry.
// Returns the number of rules loaded and any error.
func (r *Registry) LoadFromDir(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read policy dir %q: %w", dir, err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Only load .yaml and .yml files
		name := entry.Name()
		if len(name) < 5 || (name[len(name)-5:] != ".yaml" && name[len(name)-4:] != ".yml") {
			continue
		}

		data, err := os.ReadFile(fmt.Sprintf("%s/%s", dir, entry.Name()))
		if err != nil {
			return count, fmt.Errorf("read policy file %q: %w", name, err)
		}

		rules, err := LoadFromYAML(data)
		if err != nil {
			return count, fmt.Errorf("parse policy file %q: %w", name, err)
		}

		for _, rule := range rules {
			if regErr := r.Register(rule); regErr != nil {
				return count, fmt.Errorf("register rule from %q: %w", name, regErr)
			}
			count++
		}
	}

	return count, nil
}
