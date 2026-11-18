package manifest

import (
	"strings"
	"testing"
)

const head = "id: acme/pr-prep\nversion: 0.1.0\nsummary: Drafts.\n"

func TestCommandArgsFlagsAndExamplesParse(t *testing.T) {
	l, err := Parse([]byte(head+`examples:
  - ilk pr-prep draft SPEC-x
commands:
  - name: draft
    summary: Draft a description
    args:
      - {name: document, required: true, usage: the plan}
      - {name: rest, variadic: true, usage: anything else}
    flags:
      - {name: title, type: bool, usage: only the title}
      - {name: max-lines, short: m, type: int, default: "40", usage: cap the body}
    examples:
      - ilk pr-prep draft SPEC-x --title
    json: true
    run: draft.sh
`), "ok")
	if err != nil {
		t.Fatal(err)
	}
	if got := l.Examples; len(got) != 1 || got[0] != "ilk pr-prep draft SPEC-x" {
		t.Errorf("layer examples = %v", got)
	}
	c := l.Commands[0]
	if !c.JSON || len(c.Args) != 2 || len(c.Flags) != 2 || len(c.Examples) != 1 {
		t.Fatalf("command not parsed whole: %+v", c)
	}
	if got := c.ArgsUsage(); got != "<document> [rest...]" {
		t.Errorf("ArgsUsage = %q", got)
	}
	if got := c.Synopsis(); got != "<document> [rest...] [--title] [--max-lines <int>]" {
		t.Errorf("Synopsis = %q", got)
	}
	if got := c.Flags[1].EnvName(); got != "ILK_FLAG_MAX_LINES" {
		t.Errorf("EnvName = %q", got)
	}
}

func TestBadCommandDeclarationsAreRefused(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"args:\n      - {name: a}\n      - {name: b, required: true}", "cannot follow an optional"},
		{"args:\n      - {name: a, variadic: true}\n      - {name: b}", "only the last argument"},
		{"flags:\n      - {name: json, usage: x}", "ilk's own flag"},
		{"flags:\n      - {name: x, short: C, usage: x}", "ilk's own flag"},
		{"flags:\n      - {name: x, type: float, usage: x}", "not one of bool, string, int"},
		{"flags:\n      - {name: x, type: int, default: lots, usage: x}", "whole number"},
		{"flags:\n      - {name: x}", "usage is required"},
		{"examples: [\"\"]", "empty"},
	} {
		manifest := head + "commands:\n  - name: draft\n    summary: Draft\n    run: draft.sh\n    " + tc.body + "\n"
		_, err := Parse([]byte(manifest), "bad")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: want an error containing %q, got %v", tc.body, tc.want, err)
		}
	}
}

// Check ids start with the layer's name, so `ilk check --only <layer>` means
// something and two layers cannot both register `evidence`.
func TestCheckIDsMustCarryTheLayersName(t *testing.T) {
	check := func(id string) error {
		_, err := Parse([]byte(head+"checks:\n  - id: "+id+"\n    run: \"true\"\n    fix: Do it.\n"), id)
		return err
	}
	if err := check("pr-prep.criteria-settled"); err != nil {
		t.Errorf("a prefixed id should parse: %v", err)
	}
	for _, id := range []string{"prprep.criteria-settled", "criteria-settled", "pr-prep.", "pr-prepx.y"} {
		err := check(id)
		if err == nil || !strings.Contains(err.Error(), "must start with the layer's name") {
			t.Errorf("%q should be refused: %v", id, err)
		}
	}
}

func TestFindingsFormatIsChecked(t *testing.T) {
	_, err := Parse([]byte(head+"checks:\n  - id: pr-prep.x\n    run: \"true\"\n    findings: csv\n    fix: Do it.\n"), "bad")
	if err == nil || !strings.Contains(err.Error(), "jsonl") {
		t.Errorf("an unknown findings format should be refused: %v", err)
	}
	_, err = Parse([]byte(head+"checks:\n  - id: pr-prep.x\n    kind: builtin.links\n    findings: jsonl\n    fix: Do it.\n"), "bad")
	if err == nil || !strings.Contains(err.Error(), "only means something for a `run:` check") {
		t.Errorf("findings on a builtin should be refused: %v", err)
	}
}
