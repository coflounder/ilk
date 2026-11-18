package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/spf13/cobra"
)

// `ilk config` reads and writes the settings in .ilk/config.yaml that a person
// otherwise edits by hand: capabilities (`test.command`) and layer variables
// (`record.plans_dir`). Writing one is a change to the desired state, so it goes
// through the same plan, confirmation and apply as `ilk add`.
//
// A key names a layer variable when its first segment is an adopted layer's name
// and the rest is a variable that layer declares; anything else is a capability.
// Capability names cannot contain underscores and variable names usually do, so
// the two rarely look alike in practice.

const (
	kindCapability = "capability"
	kindVariable   = "variable"

	sourceConfig  = "config"
	sourceDefault = "default"
	sourceUnset   = "unset"
)

// configEntry is one setting as `ilk config list` and `get` report it.
type configEntry struct {
	Key    string   `json:"key"`
	Kind   string   `json:"kind"`
	Value  string   `json:"value"`
	Set    bool     `json:"set"`
	Source string   `json:"source"`
	Layer  string   `json:"layer,omitempty"`
	UsedBy []string `json:"used_by,omitempty"`
}

type configListDoc struct {
	Capabilities []configEntry `json:"capabilities"`
	Variables    []configEntry `json:"variables"`
}

type configChangeDoc struct {
	Key      string      `json:"key"`
	Kind     string      `json:"kind"`
	Old      *string     `json:"old"`
	New      *string     `json:"new"`
	Enables  []string    `json:"enables,omitempty"`
	Disables []string    `json:"disables,omitempty"`
	Applied  bool        `json:"applied"`
	Plan     planJSONDoc `json:"plan"`
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read and change capabilities and layer variables",
		Long: `Read and change the settings in .ilk/config.yaml without editing it by hand.

Keys are capabilities, which tell layers how this project verifies itself
(` + "`test.command`" + `, ` + "`lint.command`" + `, ` + "`build.command`" + `), and layer variables, written
<layer>.<variable> (` + "`record.plans_dir`" + `, ` + "`gates.ci`" + `). ` + "`ilk info <layer>`" + ` lists a
layer's variables.

Changing a setting reconciles the repository in the same step, so you see the plan
before anything is written. With no arguments, ` + "`ilk config`" + ` lists every setting.`,
		Example: `  ilk config
  ilk config set test.command "go test ./..."
  ilk config get test.command --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runConfigList() },
	}
	cmd.AddCommand(newConfigListCmd(), newConfigGetCmd(), newConfigSetCmd(), newConfigUnsetCmd())
	return cmd
}

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every capability and layer variable, and where each value comes from",
		Example: `  ilk config list
  ilk config list --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runConfigList() },
	}
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print one setting's value",
		Long: `Print the value a capability or layer variable has in this repository.

The value is the effective one: set in .ilk/config.yaml, supplied by a layer, or a
variable's default. A capability that a layer needs but nobody supplies exits 1;
a key nothing here knows exits 2.`,
		Example: `  ilk config get test.command
  ilk config get record.plans_dir --json`,
		Args: requireArgs(1, "ilk config get <key>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usageError(fmt.Errorf("get takes one key, got %d", len(args)))
			}
			p, err := project()
			if err != nil {
				return err
			}
			e, ok := lookupSetting(p, args[0])
			if !ok {
				return errUnknownConfigKey(args[0])
			}
			if e.Source == sourceUnset {
				return &cliError{
					Exit:    exitFailed,
					Code:    "config_key_unset",
					Message: fmt.Sprintf("%s has no value: %s needs it, and nothing supplies it", e.Key, strings.Join(e.UsedBy, ", ")),
					Fix:     fmt.Sprintf("ilk config set %s <value>", e.Key),
				}
			}
			if flagJSON {
				return emitJSON(e)
			}
			println(e.Value)
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a capability or layer variable, and reconcile the repository",
		Long: `Set a capability or a layer variable in .ilk/config.yaml.

If the new value changes any file ilk manages, you see the plan and confirm it, as
with ` + "`ilk add`" + `; with no terminal to ask on, pass --yes or nothing is written. A
change that touches no files is saved straight away.

A capability nothing here uses yet is still saved, with a note: a layer added later
may be the one that reads it.`,
		Example: `  ilk config set test.command "go test ./..."
  ilk config set lint.command "golangci-lint run" --yes
  ilk config set gates.ci none --yes`,
		Args: requireArgs(2, "ilk config set <key> <value>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 2 {
				return usageError(fmt.Errorf("set takes a key and one value, got %d arguments — quote a value that has spaces", len(args)))
			}
			key, value := args[0], args[1]
			if strings.TrimSpace(value) == "" {
				return usageError(fmt.Errorf("%s needs a value — use `ilk config unset %s` to remove it", key, key))
			}
			p, err := project()
			if err != nil {
				return err
			}
			return changeSetting(p, key, &value, yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without confirming")
	return cmd
}

func newConfigUnsetCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "unset <key>",
		Short: "Remove a setting from .ilk/config.yaml, and reconcile the repository",
		Long: `Remove a capability or layer variable from .ilk/config.yaml.

A variable goes back to its default; a capability goes back to whatever a layer
supplies, or to nothing, and the checks that need it are skipped again. A key that
is known but not set in the file is left alone.`,
		Example: `  ilk config unset lint.command
  ilk config unset record.plans_dir --yes`,
		Args: requireArgs(1, "ilk config unset <key>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usageError(fmt.Errorf("unset takes one key, got %d", len(args)))
			}
			p, err := project()
			if err != nil {
				return err
			}
			return changeSetting(p, args[0], nil, yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without confirming")
	return cmd
}

func errUnknownConfigKey(key string) error {
	return &cliError{
		Exit:    exitUsage,
		Code:    "unknown_config_key",
		Message: fmt.Sprintf("%q is not a capability or layer variable this repository knows", key),
		Fix:     "`ilk config list` shows the keys; `ilk config set <key> <value>` adds a capability",
	}
}

func runConfigList() error {
	p, err := project()
	if err != nil {
		return err
	}
	doc := configListDoc{Capabilities: capabilityEntries(p), Variables: variableEntries(p)}
	if flagJSON {
		return emitJSON(doc)
	}

	keyW, valW := 0, 0
	for _, e := range append(append([]configEntry{}, doc.Capabilities...), doc.Variables...) {
		keyW = max(keyW, len(e.Key))
		valW = max(valW, min(len(e.Value), 40))
	}
	valW = max(valW, len("(unset)"))
	row := func(e configEntry) {
		// Padding is applied to the plain value so colour codes do not skew it.
		pad := strings.Repeat(" ", max(0, valW-len(e.Value)))
		if e.Source == sourceUnset {
			pad = strings.Repeat(" ", valW-len("(unset)"))
		}
		printf("  %-*s  %s%s  %s\n", keyW, e.Key, displayValue(e), pad, sty.dim(describeSource(e)))
	}

	printf("%s\n", sty.bold("Capabilities"))
	if len(doc.Capabilities) == 0 {
		printf("  %s\n", sty.dim("none — `ilk config set test.command \"<command>\"` adds one"))
	}
	for _, e := range doc.Capabilities {
		row(e)
	}
	if len(doc.Variables) > 0 {
		printf("\n%s\n", sty.bold("Layer variables"))
		for _, e := range doc.Variables {
			row(e)
		}
	}
	return nil
}

func displayValue(e configEntry) string {
	if e.Source == sourceUnset {
		return sty.yellow("(unset)")
	}
	return e.Value
}

func describeSource(e configEntry) string {
	switch {
	case e.Source == sourceUnset:
		return "needed by " + strings.Join(e.UsedBy, ", ")
	case e.Source == sourceConfig || e.Source == sourceDefault:
		return e.Source
	default:
		return "from " + e.Source
	}
}

// capabilityEntries lists every capability the repository sets, a layer supplies,
// or a layer or check needs.
func capabilityEntries(p *engine.Project) []configEntry {
	effective := p.Capabilities()
	users := capabilityUsers(p)
	providers := map[string]string{}
	for _, l := range p.Layers {
		for _, c := range l.Loaded.Manifest.Provides.Names() {
			if _, ok := providers[c]; !ok {
				providers[c] = l.ID()
			}
		}
	}

	keys := map[string]bool{}
	for k := range effective {
		keys[k] = true
	}
	for k := range users {
		keys[k] = true
	}
	var out []configEntry
	for k := range keys {
		e := configEntry{Key: k, Kind: kindCapability, UsedBy: users[k]}
		if v, ok := p.Config.Capabilities[k]; ok {
			e.Value, e.Set, e.Source = v, true, sourceConfig
		} else if v, ok := effective[k]; ok {
			e.Value, e.Source = v, providers[k]
		} else {
			e.Source = sourceUnset
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	if out == nil {
		out = []configEntry{}
	}
	return out
}

// capabilityUsers maps each capability to the checks and layers that need it.
// A check is named in preference to its layer, because the check is what a
// capability switches on.
func capabilityUsers(p *engine.Project) map[string][]string {
	users := map[string][]string{}
	add := func(c, who string) {
		for _, existing := range users[c] {
			if existing == who {
				return
			}
		}
		users[c] = append(users[c], who)
	}
	for _, l := range p.Layers {
		m := l.Loaded.Manifest
		checked := map[string]bool{}
		for _, c := range m.Checks {
			if c.Requires != "" {
				add(c.Requires, c.ID)
				checked[c.Requires] = true
			}
		}
		for _, r := range m.Requires {
			if !checked[r] {
				add(r, l.ID())
			}
		}
	}
	return users
}

// checksEnabledBy lists the checks that run only when a capability is supplied.
func checksEnabledBy(p *engine.Project, capability string) []string {
	var ids []string
	for _, l := range p.Layers {
		for _, c := range l.Loaded.Manifest.Checks {
			if c.Requires == capability {
				ids = append(ids, c.ID)
			}
		}
	}
	return ids
}

func variableEntries(p *engine.Project) []configEntry {
	out := []configEntry{}
	for _, l := range p.Layers {
		names := make([]string, 0, len(l.Loaded.Manifest.Variables))
		for name := range l.Loaded.Manifest.Variables {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			out = append(out, variableEntry(l, name))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func variableEntry(l *engine.ResolvedLayer, name string) configEntry {
	e := configEntry{Key: l.Name() + "." + name, Kind: kindVariable, Layer: l.ID(), Value: l.Vars[name], Source: sourceDefault}
	if v, ok := l.Ref.Vars[name]; ok {
		e.Value, e.Set, e.Source = v, true, sourceConfig
	}
	return e
}

// variableFor resolves `<layer>.<variable>` to an adopted layer and one of its
// declared variables.
func variableFor(p *engine.Project, key string) (*engine.ResolvedLayer, string, bool) {
	for i := 1; i < len(key)-1; i++ {
		if key[i] != '.' {
			continue
		}
		l, ok := p.Layer(key[:i])
		if !ok {
			continue
		}
		if _, declared := l.Loaded.Manifest.Variables[key[i+1:]]; declared {
			return l, key[i+1:], true
		}
	}
	return nil, "", false
}

// lookupSetting finds a key among the settings this repository knows about.
func lookupSetting(p *engine.Project, key string) (configEntry, bool) {
	if l, name, ok := variableFor(p, key); ok {
		return variableEntry(l, name), true
	}
	for _, e := range capabilityEntries(p) {
		if e.Key == key {
			return e, true
		}
	}
	return configEntry{}, false
}

// changeSetting sets (value non-nil) or unsets a key, then plans, confirms and
// applies the result the way `ilk add` does.
func changeSetting(p *engine.Project, key string, value *string, yes bool) error {
	doc := configChangeDoc{Key: key}
	before := p.Capabilities()

	if l, name, ok := variableFor(p, key); ok {
		doc.Kind = kindVariable
		ref := l.Ref
		vars := map[string]string{}
		for k, v := range ref.Vars {
			vars[k] = v
		}
		if old, set := vars[name]; set {
			doc.Old = &old
		}
		if value == nil {
			if doc.Old == nil {
				return reportUnchanged(doc, "is not set in .ilk/config.yaml; it already has its default")
			}
			delete(vars, name)
		} else {
			if doc.Old != nil && *doc.Old == *value {
				doc.New = value
				return reportUnchanged(doc, "already has that value")
			}
			vars[name] = *value
			doc.New = value
		}
		if _, err := l.Loaded.ResolveVars(vars); err != nil {
			return usageError(err)
		}
		if len(vars) == 0 {
			vars = nil
		}
		ref.Vars = vars
		p.Config.Set(ref)
	} else {
		doc.Kind = kindCapability
		if !manifest.ValidCapability(key) {
			if value == nil {
				return errUnknownConfigKey(key)
			}
			return &cliError{
				Exit:    exitUsage,
				Code:    "invalid_config_key",
				Message: fmt.Sprintf("%q is neither a layer variable here nor a capability name", key),
				Fix:     "capabilities are dotted lowercase names like test.command; variables are <layer>.<variable>, listed by `ilk config list`",
			}
		}
		if old, set := p.Config.Capabilities[key]; set {
			doc.Old = &old
		}
		if value == nil {
			if doc.Old == nil {
				if _, known := lookupSetting(p, key); !known {
					return errUnknownConfigKey(key)
				}
				return reportUnchanged(doc, "is not set in .ilk/config.yaml")
			}
			delete(p.Config.Capabilities, key)
		} else {
			if doc.Old != nil && *doc.Old == *value {
				doc.New = value
				return reportUnchanged(doc, "already has that value")
			}
			if _, known := lookupSetting(p, key); !known {
				warnf("%s nothing in this repository uses %s yet; it is saved for a layer that will\n", sty.yellow("note:"), key)
			}
			p.Config.Capabilities[key] = *value
			doc.New = value
		}
	}

	next, err := engine.NewProject(p.Repo, p.Config, p.Lock, p.Version)
	if err != nil {
		return err
	}
	after := next.Capabilities()
	if doc.Kind == kindCapability {
		_, had := before[key]
		_, has := after[key]
		switch {
		case !had && has:
			doc.Enables = checksEnabledBy(next, key)
		case had && !has:
			doc.Disables = checksEnabledBy(next, key)
		}
	}
	pl, err := next.Plan(engine.PlanOptions{Prune: true})
	if err != nil {
		return err
	}
	doc.Plan = planJSON(pl)

	commit := func() error {
		if err := p.Config.Save(p.Repo.Root); err != nil {
			return err
		}
		if err := next.Apply(pl); err != nil {
			return err
		}
		doc.Applied = true
		if len(pl.Conflicts()) > 0 {
			exitCode = exitFailed
		}
		return nil
	}

	// Under --json, as with `ilk add` and `ilk rm`, the caller has already
	// chosen: the change is made and the result is the document.
	if flagJSON {
		if err := commit(); err != nil {
			return err
		}
		return emitJSON(doc)
	}

	printf("%s  %s → %s\n", sty.bold(key), valueOrUnset(doc.Old), valueOrUnset(doc.New))
	if len(doc.Enables) > 0 {
		printf("  %s %s\n", sty.dim("enables:"), strings.Join(doc.Enables, ", "))
	}
	if len(doc.Disables) > 0 {
		printf("  %s %s\n", sty.dim("disables:"), strings.Join(doc.Disables, ", "))
	}

	if pl.Empty() && len(pl.Conflicts()) == 0 {
		if err := commit(); err != nil {
			return err
		}
		printf("%s\n", sty.dim("saved to .ilk/config.yaml; no files change."))
		return nil
	}

	printf("\n")
	printPlan(pl, false)
	if ok, err := approve(yes, "\nApply?"); !ok {
		if err != nil {
			return errConfirmationRequired(fmt.Sprintf("review the plan above, then run `%s --yes`", configCommandLine(key, value)))
		}
		println(sty.dim("Nothing changed."))
		return nil
	}
	if err := commit(); err != nil {
		return err
	}
	printf("\n%s %s\n", sty.green("applied."), summariseCounts(pl))
	if n := len(pl.Conflicts()); n > 0 {
		warnf("%s %d artifact(s) skipped — see above.\n", sty.yellow("note:"), n)
	}
	return nil
}

func reportUnchanged(doc configChangeDoc, why string) error {
	doc.Plan = planJSONDoc{Changes: []actionJSON{}, Conflicts: []actionJSON{}, Summary: "no changes"}
	if flagJSON {
		return emitJSON(doc)
	}
	printf("%s %s\n", doc.Key, sty.dim(why+"; nothing to do."))
	return nil
}

func valueOrUnset(v *string) string {
	if v == nil {
		return sty.dim("(unset)")
	}
	return *v
}

// configCommandLine rebuilds the command a caller ran, for the exit-3 fix.
func configCommandLine(key string, value *string) string {
	if value == nil {
		return "ilk config unset " + key
	}
	return "ilk config set " + key + " " + shellQuoteAll([]string{*value})
}
