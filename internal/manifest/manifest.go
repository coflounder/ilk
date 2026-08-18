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
// rm do to it. See docs/reference/REF-design-proposal.md §3.1.
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
	Provides Provides `yaml:"provides,omitempty"`

	Variables map[string]Variable `yaml:"variables,omitempty"`

	// Literal repository files.
	Files []File `yaml:"files,omitempty"`
	// Directories to create (with a .gitkeep when empty).
	Dirs []Dir `yaml:"dirs,omitempty"`
	// Groups this layer introduces, beyond the canonical ones.
	Groups []Group `yaml:"groups,omitempty"`
	// Neutral artifacts projected per agent target. See internal/targets.
	Instructions []Instruction `yaml:"instructions,omitempty"`
	Skills       []Skill       `yaml:"skills,omitempty"`
	Hooks        []Hook        `yaml:"hooks,omitempty"`
	MCP          []MCPServer   `yaml:"mcp,omitempty"`
	// Checks contributed to `ilk check`.
	Checks []Check `yaml:"checks,omitempty"`
	// Commands extend the CLI as `ilk <layer> <command>`.
	Commands []Command `yaml:"commands,omitempty"`
	// Mirrors keep record documents in agreement with an external tracker.
	Mirrors []Mirror `yaml:"mirrors,omitempty"`
	// Contribution declares how a repository sends back what it learned.
	Contribution *Contribution `yaml:"contribution,omitempty"`

	// Source records where this layer was loaded from. Set by the loader.
	Source string `yaml:"-"`
}

// Provides maps a capability this layer supplies to the value it supplies for
// it. An empty value means the layer only declares that the capability exists.
//
// Values matter for capabilities that name a place. A planning layer needs to
// know *where* the plans are, not merely that somebody keeps some; before this,
// every such layer redeclared `plans_dir: plans` with a comment saying it must
// match the record layer, and moving the directory broke all of them silently.
// A consumer now reads `{{ index .Caps "record.plans" }}` and follows.
//
// Both spellings parse, because a capability with no value is still the common
// case and should not have to write `: ""`:
//
//	provides: [record.docs, record.plans]
//
//	provides:
//	  record.docs: "{{ .Vars.docs_dir }}"
//	  record.plans: "{{ .Vars.plans_dir }}"
//
// A value is a template over `.Vars` and `.Repo` only. It cannot read `.Caps`,
// because capability resolution is what is being computed.
type Provides map[string]string

// UnmarshalYAML accepts either a list of capability names or a mapping of names
// to values.
func (p *Provides) UnmarshalYAML(n *yaml.Node) error {
	out := Provides{}
	switch n.Kind {
	case yaml.SequenceNode:
		var names []string
		if err := n.Decode(&names); err != nil {
			return err
		}
		for _, name := range names {
			out[name] = ""
		}
	case yaml.MappingNode:
		// Decoded into a plain map, never into Provides: decoding into the named
		// type would call this method again, for ever.
		plain := map[string]string{}
		if err := n.Decode(&plain); err != nil {
			return err
		}
		for name, value := range plain {
			out[name] = value
		}
	default:
		return fmt.Errorf("provides must be a list of capabilities or a mapping of capability to value")
	}
	*p = out
	return nil
}

// MarshalYAML keeps round-tripping stable and output deterministic.
func (p Provides) MarshalYAML() (any, error) {
	if len(p) == 0 {
		return nil, nil
	}
	valued := false
	for _, v := range p {
		if v != "" {
			valued = true
		}
	}
	if !valued {
		return p.Names(), nil
	}
	return map[string]string(p), nil
}

// Names lists the capabilities in a stable order.
func (p Provides) Names() []string {
	names := make([]string, 0, len(p))
	for name := range p {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Variable is a layer input. Opinionated defaults are the point: a layer should
// land with no prompts at all.
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
//
// A directory is declared one of two ways. Grouped, with `group:` and `name:`,
// which puts it under a shared top-level heading — this is what keeps a
// repository's root from growing one folder per layer. Or free, with `path:`,
// for the few directories that genuinely belong somewhere specific.
//
// There is deliberately no ordinal in the path. Ordering is presentation: it
// decides where a directory appears in its group's generated index, and nothing
// else. Putting numbers in the paths themselves would mean layers competing for
// slots in a global integer namespace they cannot see, and every renumbering
// rewriting links and `covers:` globs across the repository.
type Dir struct {
	// Path places the directory explicitly. Mutually exclusive with group/name.
	Path string `yaml:"path,omitempty"`
	// Group is the top-level grouping this directory belongs under.
	Group string `yaml:"group,omitempty"`
	// Name is the directory's own name inside its group.
	Name string `yaml:"name,omitempty"`
	// Order sorts this directory within its group's index. Lower comes first;
	// equal orders fall back to the name. Leave gaps.
	Order   int    `yaml:"order,omitempty"`
	Purpose string `yaml:"purpose,omitempty"`
	Keep    bool   `yaml:"keep,omitempty"`   // write a .gitkeep
	Ignore  bool   `yaml:"ignore,omitempty"` // add to .gitignore
}

// Grouped reports whether the directory is declared under a group.
func (d Dir) Grouped() bool { return d.Group != "" }

// Group declares a top-level directory that gathers related directories, so that
// what a repository has is legible from its root rather than being a flat list of
// whatever each layer happened to want.
//
// A layer may only place a directory in a group that is canonical or that the
// same layer declares, so `ilk layer validate` can check the reference without a
// network and without knowing what else a repository has.
type Group struct {
	Name    string `yaml:"name"`
	Purpose string `yaml:"purpose,omitempty"`
	// Order sorts groups among themselves, for the same presentational reason
	// Dir.Order exists.
	Order int `yaml:"order,omitempty"`
}

// CanonicalGroups are the groupings ilk knows about in every repository, so that
// the common shapes mean the same thing everywhere and two unrelated layers
// naming `docs` agree about what it is.
var CanonicalGroups = []Group{
	{Name: "docs", Purpose: "The project record — what is true, what is intended, and what happened.", Order: 10},
	{Name: "infra", Purpose: "How this project is deployed and operated, as code.", Order: 20},
	{Name: "ops", Purpose: "Running the thing: runbooks, incidents, on-call.", Order: 30},
}

// IsCanonicalGroup reports whether name is one of the groups ilk ships.
func IsCanonicalGroup(name string) bool {
	for _, g := range CanonicalGroups {
		if g.Name == name {
			return true
		}
	}
	return false
}

// isTemplated reports whether a manifest value is resolved per repository rather
// than fixed by the layer.
func isTemplated(s string) bool { return strings.Contains(s, "{{") }

func canonicalGroupNames() []string {
	names := make([]string, 0, len(CanonicalGroups))
	for _, g := range CanonicalGroups {
		names = append(names, g.Name)
	}
	return names
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

// MCPServer declares an MCP server the repository's agents should have, in
// neutral form. Targets project it as `ilk mcp run <name>` rather than writing
// the command into their config, for the same reason hooks project as
// `ilk hook run`: the generated file never changes when the layer does, and
// ilk's entries stay recognisable in a file whose schema it does not own.
//
// Credentials never appear here. RequiresEnv names the environment variables a
// server needs; `ilk mcp run` tests them for presence — without reading them —
// and refuses to start with a message naming what is missing, instead of the
// agent reporting an opaque connection failure.
type MCPServer struct {
	Name    string `yaml:"name"`
	Summary string `yaml:"summary,omitempty"`
	Command string `yaml:"command"`
	// Args and Env values are templated over the layer's variables and the
	// repository's capabilities, like every other manifest value.
	Args        []string          `yaml:"args,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
	RequiresEnv []string          `yaml:"requires_env,omitempty"`
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
	// RequiresEnv names environment variables the check's command needs, and is
	// the whole of ilk's credential story.
	//
	// A credential must come from the environment and never from the repository,
	// which leaves ilk unable to tell "this failed" from "nobody was logged in" —
	// the command exits non-zero either way. A layer that says which variables
	// carry the credential lets ilk skip with a reason instead, so an absent token
	// reads as an absent token rather than as a broken repository. `ilk doctor`
	// reports every check dormant for want of one.
	//
	// The values are never read, only tested for presence. ilk has no business
	// holding a secret it was asked to detect.
	RequiresEnv []string `yaml:"requires_env,omitempty"`
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

// Contribution declares how a repository sends back what it learned about a layer.
//
// A layer that its adopters cannot improve decays quietly. Somebody edits the
// managed file, the edit works, the repository moves on, and upstream never finds
// out that its content was wrong — so the next hundred adopters make the same edit.
// This block is what closes that loop, and its absence is a layer saying "do not
// tell me".
//
// The direction back down already exists: `ilk upgrade` three-way merges an
// improved layer into a repository that has tuned it. This is the direction up.
type Contribution struct {
	// Repo is where the layer's source lives, as `owner/name`.
	Repo string `yaml:"repo"`
	// Path locates the layer inside that repository. Empty means the root.
	Path string `yaml:"path,omitempty"`
	// Base is the branch proposals target. Defaults to main.
	Base string `yaml:"base,omitempty"`
	// Guidelines names a file in the layer's own tree stating what this layer
	// wants from a proposal. It is shown to whoever is drafting one, so a
	// contributor learns the standard before writing rather than in review.
	Guidelines string `yaml:"guidelines,omitempty"`
	// Submit opens the proposal upstream. Left empty, `ilk contribute` uses the
	// default the toolkit layer ships, which drives `gh`. Overriding it is how a
	// layer hosted somewhere other than GitHub stays contributable.
	Submit string `yaml:"submit,omitempty"`
}

var (
	idPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*/[a-z0-9][a-z0-9._-]*$`)
	shortIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	namePattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	capPattern     = regexp.MustCompile(`^[a-z0-9]+(\.[a-z0-9-]+)+$`)
	repoPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`)
	envPattern     = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
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
	for _, c := range l.Provides.Names() {
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

	declaredGroups := map[string]bool{}
	seenGroup := map[string]bool{}
	for i, g := range l.Groups {
		if !namePattern.MatchString(g.Name) {
			bad("groups[%d]: name %q must be lowercase-with-dashes", i, g.Name)
		}
		if seenGroup[g.Name] {
			bad("groups[%d]: duplicate name %q", i, g.Name)
		}
		seenGroup[g.Name] = true
		if strings.TrimSpace(g.Purpose) == "" && !IsCanonicalGroup(g.Name) {
			bad("groups[%d] (%s): purpose is required — it is the one line the group's index leads with", i, g.Name)
		}
		declaredGroups[g.Name] = true
	}

	for i, d := range l.Dirs {
		switch {
		case d.Path == "" && d.Group == "" && d.Name == "":
			bad("dirs[%d]: set either `path:`, or `group:` and `name:`", i)
		case d.Path != "" && (d.Group != "" || d.Name != ""):
			bad("dirs[%d]: set `path:` or `group:`/`name:`, not both", i)
		case d.Group != "" && d.Name == "":
			bad("dirs[%d]: `group: %s` needs a `name:` for the directory itself", i, d.Group)
		case d.Name != "" && d.Group == "" && d.Path == "":
			bad("dirs[%d] (%s): a directory with no `group:` must use `path:` instead of `name:`, so it is clear it sits at the repository root", i, d.Name)
		}
		for field, value := range map[string]string{"path": d.Path, "name": d.Name} {
			if strings.HasPrefix(value, "/") || strings.Contains(value, "..") {
				bad("dirs[%d]: %s %q must be relative to the repository root", i, field, value)
			}
		}
		if strings.Contains(d.Name, "/") && !isTemplated(d.Name) {
			bad("dirs[%d]: name %q is a single directory name — use `path:` for a nested location", i, d.Name)
		}
		// A literal group reference is checked against what this manifest can see,
		// so a layer is self-contained and `ilk layer validate` needs no repository.
		//
		// A templated one cannot be: the variable it reads has no value until a
		// repository supplies one. Those are settled at plan time instead, where
		// the group is resolved and the same complaint is available with a real
		// name in it.
		if d.Group != "" && !isTemplated(d.Group) && !IsCanonicalGroup(d.Group) && !declaredGroups[d.Group] {
			bad("dirs[%d]: unknown group %q — declare it in this layer's `groups:` block, or use one of: %s",
				i, d.Group, strings.Join(canonicalGroupNames(), ", "))
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

	seenMCP := map[string]bool{}
	for i, s := range l.MCP {
		if !namePattern.MatchString(s.Name) {
			bad("mcp[%d]: name %q must be lowercase-with-dashes", i, s.Name)
		}
		if seenMCP[s.Name] {
			bad("mcp[%d]: duplicate name %q", i, s.Name)
		}
		seenMCP[s.Name] = true
		if strings.TrimSpace(s.Command) == "" {
			bad("mcp[%d] (%s): command is required — the server executable ilk starts", i, s.Name)
		}
		for _, name := range s.RequiresEnv {
			if !envPattern.MatchString(name) {
				bad("mcp[%d] (%s): requires_env %q must be an environment variable name like LINEAR_API_KEY", i, s.Name, name)
			}
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
		for _, name := range c.RequiresEnv {
			if !envPattern.MatchString(name) {
				bad("checks[%d] (%s): requires_env %q must be an environment variable name like PULUMI_ACCESS_TOKEN", i, c.ID, name)
			}
		}
		if len(c.RequiresEnv) > 0 && c.Run == "" {
			bad("checks[%d] (%s): requires_env only means something for a `run:` check — a builtin reads files, not credentials", i, c.ID)
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

	if c := l.Contribution; c != nil {
		if !repoPattern.MatchString(c.Repo) {
			bad("contribution: repo %q must be `owner/name` — a proposal has to know where to go", c.Repo)
		}
		if strings.HasPrefix(c.Path, "/") || strings.Contains(c.Path, "..") {
			bad("contribution: path %q must be a relative path inside the repository", c.Path)
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
	if len(l.Commands) > 0 || len(l.Mirrors) > 0 || len(l.MCP) > 0 {
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

// ModeSymlink is a target-only mode for an artifact that is a link to another
// artifact rather than content of its own. The desired content is the link
// target, relative to the link's own directory.
//
// It exists because several agents read the same skill from different paths, and
// writing the body once per agent means every edit has to be made in every copy.
// A link keeps one canonical body and lets each agent find it where it looks.
//
// Layers cannot declare it for the same reason they cannot declare ModeMerge:
// which paths an agent reads is the adapter's knowledge, not the layer's.
const ModeSymlink Mode = "symlink"
