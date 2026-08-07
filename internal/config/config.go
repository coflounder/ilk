// Package config reads and writes .ilk/config.yaml — the repository's declared
// desired state. `ilk add` and `ilk rm` are sugar over editing this file;
// `ilk apply` reconciles the repository to it.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileName is the config file's name inside .ilk/.
const FileName = "config.yaml"

// DefaultInstructionBudget caps the total always-on instruction cost, in tokens,
// that adopted layers may contribute. Unbounded agent instructions measurably
// hurt agent performance, so the ceiling is a check, not a suggestion.
const DefaultInstructionBudget = 1500

// Config is .ilk/config.yaml.
type Config struct {
	Version      int               `yaml:"version"`
	Targets      []string          `yaml:"targets"`
	Capabilities map[string]string `yaml:"capabilities,omitempty"`
	Layers       []LayerRef        `yaml:"layers"`
	Budget       Budget            `yaml:"budget,omitempty"`
}

// Budget holds the repository's ceilings.
type Budget struct {
	Instructions int `yaml:"instructions,omitempty"`
}

// LayerRef is one adopted layer: its resolved identity plus the choices this
// repository made about it.
type LayerRef struct {
	ID        string            `yaml:"id"`
	Version   string            `yaml:"version,omitempty"`
	Source    string            `yaml:"source,omitempty"`
	Vars      map[string]string `yaml:"vars,omitempty"`
	AllowExec bool              `yaml:"allow_exec,omitempty"`
}

// Default is the configuration `ilk init` writes: opinionated, minimal, and
// useful with no registry access.
func Default() *Config {
	return &Config{
		Version:      1,
		Targets:      []string{"claude-code"},
		Capabilities: map[string]string{},
		Layers:       []LayerRef{},
		Budget:       Budget{Instructions: DefaultInstructionBudget},
	}
}

// ErrNotInitialised is returned when a command needs a config that is not there.
var ErrNotInitialised = errors.New("this repository has no .ilk/config.yaml — run `ilk init` to create one")

// Load reads the config from a repository root.
func Load(root string) (*Config, error) {
	path := Path(root)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotInitialised
	}
	if err != nil {
		return nil, err
	}
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if c.Capabilities == nil {
		c.Capabilities = map[string]string{}
	}
	if c.Budget.Instructions == 0 {
		c.Budget.Instructions = DefaultInstructionBudget
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// Path returns the config path for a repository root.
func Path(root string) string {
	return filepath.Join(root, ".ilk", FileName)
}

// Validate rejects configurations that would produce a nonsensical plan.
func (c *Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported config version %d — this ilk understands version 1", c.Version)
	}
	seen := map[string]bool{}
	for _, l := range c.Layers {
		if l.ID == "" {
			return errors.New("layers: every entry needs an id")
		}
		if seen[l.ID] {
			return fmt.Errorf("layers: %q is listed twice", l.ID)
		}
		seen[l.ID] = true
	}
	return nil
}

// Save writes the config, preserving any comments a human added. Only the keys
// ilk manages are rewritten; unknown structure is left alone.
func (c *Config) Save(root string) error {
	path := Path(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	var out []byte
	if len(existing) > 0 {
		out, err = c.mergeInto(existing)
		if err != nil {
			return err
		}
	} else {
		out, err = c.render()
		if err != nil {
			return err
		}
	}
	return writeFileAtomic(path, out, 0o644)
}

// header explains the file to whoever opens it next, human or agent.
const header = `# ilk — declared process layers for this repository.
#
# This file is the desired state. ` + "`ilk apply`" + ` reconciles the repository to it.
# ` + "`ilk add <layer>`" + ` and ` + "`ilk rm <layer>`" + ` edit it for you, but editing it
# by hand and running ` + "`ilk apply`" + ` works exactly the same way.
#
# Capabilities tell layers how this project verifies itself. Layers require
# capabilities rather than each other, so a gate layer works in any language:
#
#   capabilities:
#     test.command: go test ./...
#     lint.command: golangci-lint run
#     build.command: go build ./...
`

func (c *Config) render() ([]byte, error) {
	var buf strings.Builder
	buf.WriteString(header)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// mergeInto rewrites only ilk's own keys in an existing document, so comments
// and hand-written formatting elsewhere survive.
func (c *Config) mergeInto(existing []byte) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(existing, &doc); err != nil {
		return c.render()
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return c.render()
	}
	root := doc.Content[0]

	fresh, err := toNode(c)
	if err != nil {
		return nil, err
	}

	for i := 0; i+1 < len(fresh.Content); i += 2 {
		key := fresh.Content[i].Value
		setKey(root, key, fresh.Content[i+1])
	}
	// Drop managed keys that no longer have a value (e.g. capabilities emptied).
	managed := map[string]bool{"version": true, "targets": true, "capabilities": true, "layers": true, "budget": true}
	freshKeys := map[string]bool{}
	for i := 0; i+1 < len(fresh.Content); i += 2 {
		freshKeys[fresh.Content[i].Value] = true
	}
	for i := 0; i+1 < len(root.Content); {
		k := root.Content[i].Value
		if managed[k] && !freshKeys[k] {
			root.Content = append(root.Content[:i], root.Content[i+2:]...)
			continue
		}
		i += 2
	}

	var buf strings.Builder
	lead := leadingComments(existing)
	if lead == "" {
		lead = header
	}
	buf.WriteString(lead)
	// yaml.v3 attaches the file's leading comment block to the first node it
	// decoded. Since the block is written above by hand, clear it here or every
	// save stacks another copy on top.
	root.HeadComment = ""
	if len(root.Content) > 0 {
		root.Content[0].HeadComment = ""
	}

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// leadingComments captures the comment block at the top of the file, which
// yaml.Node round-tripping does not reliably preserve.
func leadingComments(data []byte) string {
	var out strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out.WriteString(line)
			out.WriteString("\n")
			continue
		}
		break
	}
	return out.String()
}

func toNode(v any) (*yaml.Node, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		return nil, errors.New("empty yaml document")
	}
	return doc.Content[0], nil
}

func setKey(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			// Preserve the comment attached to the key.
			value.HeadComment = mapping.Content[i+1].HeadComment
			value.LineComment = mapping.Content[i+1].LineComment
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value)
}

// Layer returns the adopted layer with the given id.
func (c *Config) Layer(id string) (LayerRef, bool) {
	for _, l := range c.Layers {
		if l.ID == id {
			return l, true
		}
	}
	return LayerRef{}, false
}

// Set adds or replaces a layer reference, keeping the list sorted so that
// generated output (and diffs) stay deterministic. `ilk add` and `ilk upgrade`
// both land here — the second one always replacing.
func (c *Config) Set(ref LayerRef) {
	for i, l := range c.Layers {
		if l.ID == ref.ID {
			c.Layers[i] = ref
			return
		}
	}
	c.Layers = append(c.Layers, ref)
	sort.Slice(c.Layers, func(i, j int) bool { return c.Layers[i].ID < c.Layers[j].ID })
}

// Remove deletes a layer reference. It reports whether anything was removed.
func (c *Config) Remove(id string) bool {
	for i, l := range c.Layers {
		if l.ID == id {
			c.Layers = append(c.Layers[:i], c.Layers[i+1:]...)
			return true
		}
	}
	return false
}

// HasTarget reports whether a target is enabled.
func (c *Config) HasTarget(name string) bool {
	for _, t := range c.Targets {
		if t == name {
			return true
		}
	}
	return false
}

// AddTarget enables a target if it is not already enabled.
func (c *Config) AddTarget(name string) bool {
	if c.HasTarget(name) {
		return false
	}
	c.Targets = append(c.Targets, name)
	sort.Strings(c.Targets)
	return true
}

// RemoveTarget disables a target.
func (c *Config) RemoveTarget(name string) bool {
	for i, t := range c.Targets {
		if t == name {
			c.Targets = append(c.Targets[:i], c.Targets[i+1:]...)
			return true
		}
	}
	return false
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".ilk-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
