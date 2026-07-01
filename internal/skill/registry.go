package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// SkillFile is the canonical skill definition filename.
const SkillFile = "SKILL.md"

// Registry holds the loaded skills, keyed by name.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]*Skill
}

// NewRegistry creates an empty skill registry.
func NewRegistry() *Registry {
	return &Registry{skills: map[string]*Skill{}}
}

// Register adds a skill. A later registration with the same name is rejected so
// the first source (registration order) wins — callers load trusted dirs first.
func (r *Registry) Register(s *Skill) error {
	if s == nil || strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("skill: cannot register a nameless skill")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.skills[s.Name]; exists {
		return fmt.Errorf("skill %q already registered", s.Name)
	}
	r.skills[s.Name] = s
	return nil
}

// Resolve returns the skill with the given name.
func (r *Registry) Resolve(name string) (*Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	if !ok {
		return nil, fmt.Errorf("unknown skill %q", name)
	}
	return s, nil
}

// List returns skill listing info, sorted by name (cheap: name+description only).
func (r *Registry) List() []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Info, 0, len(r.skills))
	for _, s := range r.skills {
		out = append(out, s.Info())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Len returns the number of registered skills.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.skills)
}

// LoadDir discovers skills under root using the Claude Code / Agent Skills layout
// (<root>/<name>/SKILL.md) and registers them with the given source label. A
// missing root is not an error (returns 0 loaded). Per-skill parse failures are
// collected and returned but do not abort the rest; successfully parsed skills are
// registered. Returns the count registered.
func (r *Registry) LoadDir(root, source string) (int, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("skill: read dir %s: %w", root, err)
	}
	var errs []string
	loaded := 0
	for _, e := range entries {
		var skillPath, skillDir string
		switch {
		case e.IsDir():
			skillDir = filepath.Join(root, e.Name())
			skillPath = filepath.Join(skillDir, SkillFile)
		case strings.EqualFold(e.Name(), SkillFile):
			// A SKILL.md placed directly in root.
			skillDir = root
			skillPath = filepath.Join(root, e.Name())
		default:
			continue
		}
		data, rerr := os.ReadFile(skillPath)
		if rerr != nil {
			continue // no SKILL.md in this subdir — skip silently
		}
		s, perr := Parse(data, skillDir, source)
		if perr != nil {
			errs = append(errs, perr.Error())
			continue
		}
		if rerr := r.Register(s); rerr != nil {
			errs = append(errs, rerr.Error())
			continue
		}
		loaded++
	}
	if len(errs) > 0 {
		return loaded, fmt.Errorf("skill: %d issue(s) loading %s: %s", len(errs), root, strings.Join(errs, "; "))
	}
	return loaded, nil
}

// PromptSuffix renders the "available skills" advertisement injected into an
// agent's system prompt, so the model knows which skills exist (name + one-line
// description) and can invoke one with the use_skill tool. Returns "" when there
// are no skills.
func PromptSuffix(infos []Info) string {
	if len(infos) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nAvailable skills (invoke with the use_skill tool when a description matches the task):\n")
	for _, i := range infos {
		desc := i.Description
		if len(desc) > 200 {
			desc = desc[:200] + "…"
		}
		fmt.Fprintf(&b, "- %s: %s\n", i.Name, desc)
	}
	return b.String()
}

// DefaultSkillDirs returns the conventional skill directories to scan, relative to
// a workspace/repo root: Claude Code skills (.claude/skills) and OpenClaw skills
// (.openclaw/skills and a top-level skills/). Each is paired with a source label.
func DefaultSkillDirs(root string) []struct{ Dir, Source string } {
	return []struct{ Dir, Source string }{
		{filepath.Join(root, ".claude", "skills"), "claude"},
		{filepath.Join(root, ".openclaw", "skills"), "openclaw"},
		{filepath.Join(root, "skills"), "workspace"},
	}
}

// LoadDefault loads skills from all DefaultSkillDirs under root, in order (so
// Claude Code skills take precedence on name collisions). Returns the total count.
func (r *Registry) LoadDefault(root string) (int, error) {
	var errs []string
	total := 0
	for _, d := range DefaultSkillDirs(root) {
		n, err := r.LoadDir(d.Dir, d.Source)
		total += n
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return total, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return total, nil
}
