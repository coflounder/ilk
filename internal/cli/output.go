package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/coflounder/ilk/internal/engine"
)

// out is the writer every command prints to, so tests can capture it.
var out io.Writer = os.Stdout

// errOut is where diagnostics go.
var errOut io.Writer = os.Stderr

type style struct{ enabled bool }

var sty = style{enabled: colorEnabled()}

func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func (s style) wrap(code, text string) string {
	if !s.enabled {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s style) bold(t string) string   { return s.wrap("1", t) }
func (s style) dim(t string) string    { return s.wrap("2", t) }
func (s style) green(t string) string  { return s.wrap("32", t) }
func (s style) red(t string) string    { return s.wrap("31", t) }
func (s style) yellow(t string) string { return s.wrap("33", t) }
func (s style) cyan(t string) string   { return s.wrap("36", t) }

func printf(format string, args ...any) { fmt.Fprintf(out, format, args...) }
func println(args ...any)               { fmt.Fprintln(out, args...) }

// emitJSON writes a value as indented JSON, which is what every `--json` flag
// produces. Agents are first-class consumers of this tool, not an afterthought.
func emitJSON(v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// opSymbol renders a plan action as a one-character diff marker.
func opSymbol(op engine.Op) string {
	switch op {
	case engine.OpCreate, engine.OpMkdir, engine.OpRegionAdd:
		return sty.green("+")
	case engine.OpUpdate, engine.OpRegionUpdate, engine.OpChmod, engine.OpVacate:
		return sty.cyan("~")
	case engine.OpDelete, engine.OpRegionRemove, engine.OpRmdir:
		return sty.red("-")
	case engine.OpConflict:
		return sty.red("!")
	case engine.OpSkip:
		return sty.dim("·")
	}
	return " "
}

func opLabel(op engine.Op) string {
	switch op {
	case engine.OpMkdir:
		return "mkdir"
	case engine.OpCreate:
		return "create"
	case engine.OpUpdate:
		return "update"
	case engine.OpRegionAdd:
		return "block +"
	case engine.OpRegionUpdate:
		return "block ~"
	case engine.OpRegionRemove:
		return "block -"
	case engine.OpDelete:
		return "delete"
	case engine.OpVacate:
		return "vacate"
	case engine.OpRmdir:
		return "rmdir"
	case engine.OpChmod:
		return "chmod"
	case engine.OpConflict:
		return "CONFLICT"
	case engine.OpSkip:
		return "skip"
	}
	return string(op)
}

// printPlan renders a plan the way a human reads a diff: what changes, who owns
// it, and why anything was refused.
func printPlan(pl *engine.Plan, showUnchanged bool) {
	changes := pl.Changes()
	conflicts := pl.Conflicts()

	if len(changes) == 0 && len(conflicts) == 0 {
		println(sty.dim("No changes. The repository already matches .ilk/config.yaml."))
	}

	for _, a := range pl.Actions {
		if a.Op == engine.OpUnchanged && !showUnchanged {
			continue
		}
		// A skipped removal is information: it says what ilk deliberately left
		// behind. A skipped write is just noise.
		if a.Op == engine.OpSkip && !showUnchanged && !a.Removal {
			continue
		}
		target := a.Path
		if a.Region != "" {
			target += sty.dim(" [" + a.Region + "]")
		}
		printf("  %s %-10s %-44s %s\n", opSymbol(a.Op), opLabel(a.Op), target, sty.dim(a.Owner))
		if a.Note != "" {
			printf("      %s\n", sty.dim(a.Note))
		}
	}

	for _, w := range pl.Warnings {
		printf("\n  %s %s\n", sty.yellow("warning"), w)
	}

	if len(conflicts) > 0 {
		printf("\n  %s %d file(s) left untouched because your edits would be lost.\n",
			sty.red("refused:"), len(conflicts))
		printf("  %s\n", sty.dim("Re-run with --force to discard those edits, or move them outside the ilk markers."))
	}
}

// summariseCounts renders "3 created, 1 updated" style tallies.
func summariseCounts(pl *engine.Plan) string {
	counts := map[engine.Op]int{}
	for _, a := range pl.Changes() {
		counts[a.Op]++
	}
	var parts []string
	for _, op := range []engine.Op{engine.OpCreate, engine.OpUpdate, engine.OpRegionAdd, engine.OpRegionUpdate, engine.OpRegionRemove, engine.OpDelete, engine.OpMkdir, engine.OpRmdir, engine.OpVacate, engine.OpChmod} {
		if n := counts[op]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, opLabel(op)))
		}
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, ", ")
}

// fail prints an error the way ilk's checks do: what went wrong, then what to do.
func fail(err error) {
	fmt.Fprintf(errOut, "%s %s\n", sty.red("error:"), err.Error())
}
