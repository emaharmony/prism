// configedit.go provides surgical, comment-preserving edits to prism.yaml so a
// UI (or any caller) can change one section without rewriting the whole file.
//
// The problem: prism.yaml is loaded by overlaying onto DefaultConfig(), so a
// naive "load Config → marshal → write" round-trip would bloat the file with
// every default value and drop all comments. Instead we edit the YAML *node
// tree* in place: only the targeted key's value is replaced, and yaml.v3
// preserves comments and key order on every node we don't touch.
package orchestrator

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// SetYAMLPath returns src with the mapping key at path set to value, creating
// intermediate mappings as needed. Comments and ordering on untouched nodes are
// preserved. An empty/whitespace src starts from an empty document.
//
// Example: SetYAMLPath(data, []string{"prism", "scheduler"}, schedulerCfg).
func SetYAMLPath(src []byte, path []string, value any) ([]byte, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("configedit: empty path")
	}

	var doc yaml.Node
	if len(src) > 0 {
		if err := yaml.Unmarshal(src, &doc); err != nil {
			return nil, fmt.Errorf("configedit: parse source yaml: %w", err)
		}
	}

	// Normalize to a document node wrapping a single mapping node.
	root := documentRoot(&doc)

	// Build the replacement value node from `value`.
	valueNode, err := toNode(value)
	if err != nil {
		return nil, fmt.Errorf("configedit: encode value: %w", err)
	}

	if err := setInMapping(root, path, valueNode); err != nil {
		return nil, err
	}

	// Encode with 2-space indent to match prism.yaml's convention (yaml.v3's
	// default is 4), keeping the diff to just the edited keys.
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("configedit: re-encode: %w", err)
	}
	enc.Close()
	return buf.Bytes(), nil
}

// documentRoot returns the top-level mapping node of doc, initializing an empty
// mapping when the document is empty (e.g. src was blank).
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == 0 || len(doc.Content) == 0 {
		doc.Kind = yaml.DocumentNode
		mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		doc.Content = []*yaml.Node{mapping}
		return mapping
	}
	return doc.Content[0]
}

// toNode marshals an arbitrary Go value into a yaml value node (unwrapping the
// document node yaml.Marshal produces).
func toNode(value any) (*yaml.Node, error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		// Empty value (e.g. nil) — represent as an explicit null.
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	}
	return doc.Content[0], nil
}

// setInMapping walks/creates the mapping path under root and sets the final
// key's value node.
func setInMapping(root *yaml.Node, path []string, value *yaml.Node) error {
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("configedit: expected mapping at document root, got kind %d", root.Kind)
	}
	current := root
	for i, key := range path {
		last := i == len(path)-1
		keyNode, valNode := findMapEntry(current, key)
		if last {
			if valNode == nil {
				current.Content = append(current.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
					value)
			} else {
				// Replace value in place, keeping any comment on the key node.
				*valNode = *value
			}
			return nil
		}
		// Intermediate: descend into an existing mapping or create one.
		if valNode == nil {
			newMap := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			current.Content = append(current.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
				newMap)
			current = newMap
			continue
		}
		if valNode.Kind != yaml.MappingNode {
			return fmt.Errorf("configedit: key %q at %v is not a mapping", key, path[:i+1])
		}
		_ = keyNode
		current = valNode
	}
	return nil
}

// findMapEntry returns the key node and value node for key in a mapping node,
// or (nil, nil) if absent. Mapping content alternates key,value,key,value…
func findMapEntry(mapping *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i], mapping.Content[i+1]
		}
	}
	return nil, nil
}

// ValidateAndWrite validates newBytes as a full prism.yaml (via the same loader
// serve uses) and, only on success, writes it atomically to path (temp file +
// rename). A validation failure returns an error and leaves path untouched.
func ValidateAndWrite(path string, newBytes []byte) error {
	if _, err := LoadConfigFromBytes(newBytes); err != nil {
		return fmt.Errorf("configedit: refusing to write invalid config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, newBytes, 0o644); err != nil {
		return fmt.Errorf("configedit: write temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("configedit: rename: %w", err)
	}
	return nil
}
