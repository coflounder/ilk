// Package manifest defines the layer schema: what a layer may contribute to a
// repository, and the validation that keeps a broken layer from ever reaching
// the plan stage.
package manifest

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Mode determines who owns a file ilk writes, and therefore what upgrade and
// drop do to it. See docs/PROPOSAL.md §3.1.
type Mode string

const (
	// ModeManaged means ilk owns the whole file: upgrades overwrite it and drop
	// deletes it.
	ModeManaged Mode = "managed"
	// ModeRegion means ilk owns a fenced block inside a file a human owns.
	ModeRegion Mode = "region"
	// ModeCreateOnly seeds a starting point ilk never touches again.
	ModeCreateOnly Mode = "create-only"
	// ModeAppendOnce adds a marked block once, idempotently.
	ModeAppendOnce Mode = "append-once"
)

var validModes = map[Mode]bool{
	ModeManaged: true, ModeRegion: true, ModeCreateOnly: true, ModeAppendOnce: true,
}

// Layer is a parsed layer.yaml.
type Layer struct {
	ID      string            `yaml:"id"`
	Version string            `yaml:"version"`
	Summary string            `yaml:"summary"`
	Facets  map[string]string `yaml:"facets,omitempty"`
	Ilk     string            `yaml:"ilk,omitempty"`

	Requires []string `yaml:"requires,omitempty"`
	Provides []string `yaml:"provides,omitempty"`

	Variables map[string]Variable `yaml:"variables,omitempty"`

	// Literal repository files.
	Files []File `yaml:"files,omitempty"`
	// Directories to create (with a .gitkeep when empty).
	Dirs []Dir `yaml:"dirs,omitempty"`
	// Neutral artifacts projected per agent target. See internal/targets.
	Instructions []Instruction `yaml:"instructions,omitempty"`
	Skills       []Skill       `yaml:"skills,omitempty"`
	Hooks        []Hook        `yaml:"hooks,omitempty"`
	// Checks contributed to `ilk check`.
	Checks []Check `yaml:"checks,omitempty"`
	// Commands extend the CLI as `ilk <layer> <command>`.
	Commands []Command `yaml:"commands,omitempty"`
	// Mirrors keep record documents in agreement with an external tracker.
	Mirrors []Mirror `yaml:"mirrors,omitempty"`

	// Source records where this layer was loaded from. Set by the loader.
	Source string `yaml:"-"`
}

// Variable is a layer input. Opinionated defaults are the point: a layer should
// be adoptable with no prompts at all.
type Variable struct {
	Default     string   `yaml:"default"`
	Prompt      string   `yaml:"prompt,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Enum        []string `yaml:"enum,omitempty"`
}

// File is a literal file written into the repository.
type File struct {
	Src    string `yaml:"src,omitempty"`
	Inline string `yaml:"inline,omitempty"`
	Dest   string `yaml:"dest"`
	Mode   Mode   `yaml:"mode"`
	Region string `yaml:"region,omitempty"`
	Exec   bool   `yaml:"exec,omitempty"`
	// When set, the file is only written if the expression evaluates truthy.
	When string `yaml:"when,omitempty"`
}

// Dir is a directory contract: a folder the layer guarantees exists.
type Dir struct {
	Path    string `yaml:"path"`
	Purpose string `yaml:"purpose,omitempty"`
	Keep    bool   `yaml:"keep,omitempty"`   // write a .gitkeep
	Ignore  bool   `yaml:"ignore,omitempty"` // add to .gitignore
}

// Instruction is a contribution to the always-on agent instructions. Budget is
// the layer's declared token cost; `ilk check` fails when the repo total crosses
// the configured ceiling, because an unbounded AGENTS.md makes agents worse.
type Instruction struct {
	ID     string `yaml:"id"`
	Src    string `yaml:"src,omitempty"`
	Inline string `yaml:"inline,omitempty"`
	Budget int    `yaml:"budget,omitempty"`
}

// Skill is on-demand guidance, loaded only when the task calls for it. Detail
// belongs here rather than in Instruction.
type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Src         string `yaml:"src,omitempty"`
	Inline      string `yaml:"inline,omitempty"`
}

// Hook binds a command to a lifecycle event. Events are neutral; each target
// declares which ones it can deliver, and `ilk doctor` reports the gaps.
type Hook struct {
	Event string `yaml:"event"`
	Run   string `yaml:"run"`
	Name  string `yaml:"name,omitempty"`
	// Blocking hooks fail the action (commit, push) when the command exits
	// non-zero. Non-blocking hooks only report.
	Blocking bool `yaml:"blocking,omitempty"`
}

// Events ilk understands. git-hook events are enforceable everywhere; agent
// events are an optimisation layered on top.
var Events = []string{"session-start", "pre-commit", "pre-push", "post-edit", "pre-tool-use"}

func ValidEvent(e string) bool {
	for _, k := range Events {
		if k == e {
			return true
		}
	}
	return false
}

// Check is a validator contributed to `ilk check`. Fix is mandatory: a check
// that cannot tell an agent how to repair the failure is a bug, not a check.
type Check struct {
	ID    string         `yaml:"id"`
	Kind  string         `yaml:"kind,omitempty"`
	Run   string         `yaml:"run,omitempty"`
	Fix   string         `yaml:"fix"`
	Args  map[string]any `yaml:"args,omitempty"`
	Title string         `yaml:"title,omitempty"`
	// Requires names a capability; the check is skipped (not failed) when the
	// repository does not supply it.
	Requires string `yaml:"requires,omitempty"`
}

// Command extends the CLI surface as `ilk <layer> <name>`.
type Command struct {
	Name    string `yaml:"name"`
	Summary string `yaml:"summary"`
	Run     string `yaml:"run"`
}

// Mirror declares that a set of record documents has a counterpart in an
// external system — a GitHub Project, a Linear team, a Jira board.
//
// ilk supplies the part that is the same everywhere: identity, diffing, refusing
// on ambiguity, and the plan-then-apply discipline that makes a write to somebody
// else's system either correct or absent. The layer supplies three commands that
// know the provider, and normalises it to a shape ilk can reason about — so ilk
// never learns what a GitHub Project is, exactly as it never learns what pytest is.
type Mirror struct {
	ID      string `yaml:"id"`
	Summary string `yaml:"summary"`
	// Dir holds the record documents to mirror.
	Dir string `yaml:"dir"`
	// Match filters those documents by filename.
	Match string `yaml:"match,omitempty"`
	// Key is the frontmatter key ilk owns on each document, holding the remote
	// identity. Everything else in the document belongs to whoever wrote it —
	// the mutex between human prose and machine output, expressed as a key.
	Key string `yaml:"key"`

	// List prints the remote items as a JSON array of
	// {id, title, status, url}. Normalising in the layer is what keeps every
	// provider's peculiarities out of ilk.
	List string `yaml:"list"`
	// Create receives one item as JSON on stdin and prints the new remote id.
	Create string `yaml:"create"`
	// Update receives {id, title, status} as JSON on stdin.
	Update string `yaml:"update"`
}

var (
	idPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*/[a-z0-9][a-z0-9._-]*$`)
	shortIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	namePattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	capPattern     = regexp.MustCompile(`^[a-z0-9]+(\.[a-z0-9-]+)+$`)
)

// Parse decodes and validates a layer manifest.
func Parse(data []byte, source string) (*Layer, error) {
	var l Layer
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&l); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	l.Source = source
	if err := l.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	return &l, nil
}

// Name is the layer's short name — the last path segment of its ID, and the
// word used to dispatch `ilk <layer> <command>`.
func (l *Layer) Name() string {
	if _, after, ok := strings.Cut(l.ID, "/"); ok {
		return after
	}
	return l.ID
}

// Validate enforces the manifest contract. Errors name the field and say what to
// do about it, for the same reason checks do.
func (l *Layer) Validate() error {
	var errs []string
	bad := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	if l.ID == "" {
		bad("id is required (e.g. `id: acme/quality-gates`)")
	} else if !idPattern.MatchString(l.ID) {
		bad("id %q must be `namespace/name` in lowercase (e.g. `acme/quality-gates`)", l.ID)
	}
	if l.Version == "" {
		bad("version is required (e.g. `version: 0.1.0`)")
	} else if !versionPattern.MatchString(l.Version) {
		bad("version %q must be semver (e.g. 0.1.0)", l.Version)
	}
	if strings.TrimSpace(l.Summary) == "" {
		bad("summary is required — it is what `ilk search` shows")
	}

	for _, c := range l.Requires {
		if !capPattern.MatchString(c) {
			bad("requires: %q must be a dotted capability like `test.command`, not a layer name", c)
		}
	}
	for _, c := range l.Provides {
		if !capPattern.MatchString(c) {
			bad("provides: %q must be a dotted capability like `record.docs`", c)
		}
	}

	for name := range l.Variables {
		if !shortIDPattern.MatchString(name) {
			bad("variables: %q must be lowercase", name)
		}
	}

	seenDest := map[string]bool{}
	for i, f := range l.Files {
		where := fmt.Sprintf("files[%d]", i)
		if f.Dest == "" {
			bad("%s: dest is required", where)
		}
		if strings.HasPrefix(f.Dest, "/") || strings.Contains(f.Dest, "..") {
			bad("%s: dest %q must be a relative path inside the repository", where, f.Dest)
		}
		if f.Src == "" && f.Inline == "" {
			bad("%s: set either src (a template file) or inline (literal content)", where)
		}
		if f.Src != "" && f.Inline != "" {
			bad("%s: set src or inline, not both", where)
		}
		if f.Mode == "" {
			bad("%s: mode is required (one of managed, region, create-only, append-once)", where)
		} else if !validModes[f.Mode] {
			bad("%s: unknown mode %q", where, f.Mode)
		}
		if (f.Mode == ModeRegion || f.Mode == ModeAppendOnce) && f.Region == "" {
			bad("%s: mode %s requires a `region:` name so ilk can find its block again", where, f.Mode)
		}
		if f.Mode == ModeManaged || f.Mode == ModeCreateOnly {
			if seenDest[f.Dest] {
				bad("%s: dest %q is written twice by this layer", where, f.Dest)
			}
			seenDest[f.Dest] = true
		}
	}

	for i, d := range l.Dirs {
		if d.Path == "" {
			bad("dirs[%d]: path is required", i)
		}
		if strings.HasPrefix(d.Path, "/") || strings.Contains(d.Path, "..") {
			bad("dirs[%d]: path %q must be relative to the repository root", i, d.Path)
		}
	}

	seenInstr := map[string]bool{}
	for i, ins := range l.Instructions {
		if ins.ID == "" {
			bad("instructions[%d]: id is required", i)
		} else if seenInstr[ins.ID] {
			bad("instructions[%d]: duplicate id %q", i, ins.ID)
		}
		seenInstr[ins.ID] = true
		if ins.Src == "" && ins.Inline == "" {
			bad("instructions[%d]: set either src or inline", i)
		}
	}

	seenSkill := map[string]bool{}
	for i, s := range l.Skills {
		if !namePattern.MatchString(s.Name) {
			bad("skills[%d]: name %q must be lowercase-with-dashes", i, s.Name)
		}
		if seenSkill[s.Name] {
			bad("skills[%d]: duplicate name %q", i, s.Name)
		}
		seenSkill[s.Name] = true
		if strings.TrimSpace(s.Description) == "" {
			bad("skills[%d] (%s): description is required — it is how an agent decides to load the skill", i, s.Name)
		}
		if s.Src == "" && s.Inline == "" {
			bad("skills[%d] (%s): set either src or inline", i, s.Name)
		}
	}

	for i, h := range l.Hooks {
		if !ValidEvent(h.Event) {
			bad("hooks[%d]: unknown event %q (one of: %s)", i, h.Event, strings.Join(Events, ", "))
		}
		if strings.TrimSpace(h.Run) == "" {
			bad("hooks[%d]: run is required", i)
		}
	}

	seenCheck := map[string]bool{}
	for i, c := range l.Checks {
		if c.ID == "" {
			bad("checks[%d]: id is required", i)
		} else if seenCheck[c.ID] {
			bad("checks[%d]: duplicate id %q", i, c.ID)
		}
		seenCheck[c.ID] = true
		if c.Kind == "" && c.Run == "" {
			bad("checks[%d] (%s): set either kind (a builtin) or run (a command)", i, c.ID)
		}
		if c.Kind != "" && c.Run != "" {
			bad("checks[%d] (%s): set kind or run, not both", i, c.ID)
		}
		if strings.TrimSpace(c.Fix) == "" {
			bad("checks[%d] (%s): fix is required — a check that cannot tell an agent how to repair the failure is not usable", i, c.ID)
		}
	}

	seenCmd := map[string]bool{}
	for i, c := range l.Commands {
		if !namePattern.MatchString(c.Name) {
			bad("commands[%d]: name %q must be lowercase-with-dashes", i, c.Name)
		}
		if seenCmd[c.Name] {
			bad("commands[%d]: duplicate name %q", i, c.Name)
		}
		seenCmd[c.Name] = true
		if strings.TrimSpace(c.Run) == "" {
			bad("commands[%d] (%s): run is required", i, c.Name)
		}
		if strings.TrimSpace(c.Summary) == "" {
			bad("commands[%d] (%s): summary is required — it is the command's help text", i, c.Name)
		}
	}

	seenMirror := map[string]bool{}
	for i, m := range l.Mirrors {
		where := fmt.Sprintf("mirrors[%d]", i)
		if !namePattern.MatchString(m.ID) {
			bad("%s: id %q must be lowercase-with-dashes", where, m.ID)
		}
		if seenMirror[m.ID] {
			bad("%s: duplicate id %q", where, m.ID)
		}
		seenMirror[m.ID] = true
		if strings.TrimSpace(m.Summary) == "" {
			bad("%s (%s): summary is required — it names the system being mirrored", where, m.ID)
		}
		if m.Dir == "" {
			bad("%s (%s): dir is required", where, m.ID)
		}
		if !shortIDPattern.MatchString(m.Key) {
			bad("%s (%s): key %q must be a lowercase frontmatter key that ilk will own on every mirrored document", where, m.ID, m.Key)
		}
		for name, value := range map[string]string{"list": m.List, "create": m.Create, "update": m.Update} {
			if strings.TrimSpace(value) == "" {
				bad("%s (%s): %s is required", where, m.ID, name)
			}
		}
	}

	if len(errs) == 0 {
		return nil
	}
	sort.Strings(errs)
	return fmt.Errorf("invalid layer manifest:\n  - %s", strings.Join(errs, "\n  - "))
}

// NeedsExec reports whether adopting this layer runs code beyond rendering
// files. Adopting such a layer requires explicit consent.
func (l *Layer) NeedsExec() bool {
	if len(l.Commands) > 0 || len(l.Mirrors) > 0 {
		return true
	}
	for _, c := range l.Checks {
		if c.Run != "" {
			return true
		}
	}
	for _, f := range l.Files {
		if f.Exec {
			return true
		}
	}
	return false
}

// Budget totals the layer's declared always-on instruction cost in tokens.
func (l *Layer) Budget() int {
	total := 0
	for _, ins := range l.Instructions {
		total += ins.Budget
	}
	return total
}

// ModeMerge is a target-only mode for files whose format has no comment syntax
// to fence — JSON, principally. ilk co-owns such a file: it inserts its own
// entries and can strip exactly those entries again, but never deletes the file.
// Layers cannot declare it, because merging requires format-specific code.
const ModeMerge Mode = "merge"
