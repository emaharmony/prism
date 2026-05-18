package adapter

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Manifest describes an adapter's capabilities from YAML.
// Manifests are optional — use them for declarative discovery.
// Register adapters programmatically for built-ins and tests.
type Manifest struct {
	Name         string       `yaml:"name" json:"name"`
	Version      string       `yaml:"version" json:"version"`
	Description  string       `yaml:"description" json:"description"`
	Capabilities []Capability `yaml:"capabilities" json:"capabilities"`
}

// LoadManifestFromYAML parses an adapter manifest from YAML bytes.
func LoadManifestFromYAML(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("adapter manifest: invalid YAML: %w", err)
	}

	if m.Name == "" {
		return nil, fmt.Errorf("adapter manifest: name is required")
	}
	if m.Version == "" {
		return nil, fmt.Errorf("adapter manifest: version is required")
	}
	if err := ValidateAdapterName(m.Name); err != nil {
		return nil, fmt.Errorf("adapter manifest: %w", err)
	}

	return &m, nil
}

// LoadManifestFromDir loads all adapter manifests from a directory.
// Returns the manifests found and any error.
func LoadManifestFromDir(dir string) ([]Manifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read adapter manifest dir %q: %w", dir, err)
	}

	var manifests []Manifest
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 5 || (name[len(name)-5:] != ".yaml" && name[len(name)-4:] != ".yml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return manifests, fmt.Errorf("read manifest %q: %w", name, err)
		}

		m, err := LoadManifestFromYAML(data)
		if err != nil {
			return manifests, fmt.Errorf("parse manifest %q: %w", name, err)
		}

		manifests = append(manifests, *m)
	}

	return manifests, nil
}