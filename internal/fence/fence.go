// Package fence implements ilk's ownership markers: the syntax that lets ilk own
// a block inside a file whose surrounding content belongs to a human.
//
// This is the "fence rule" — decide which parts of a document a machine may
// rewrite, and mark them off. It is what makes `ilk rm` non-destructive: ilk
// removes exactly the lines it wrote and leaves everything else untouched.
package fence

import (
	"bufio"
	"fmt"
	"path/filepath"
	"strings"
)

// Style is the comment syntax used to render markers in a given file type.
type Style struct {
	Open  string
	Close string
}

func (s Style) wrap(body string) string {
	if s.Close == "" {
		return s.Open + " " + body
	}
	return s.Open + " " + body + " " + s.Close
}

var (
	styleHTML  = Style{Open: "<!--", Close: "-->"}
	styleHash  = Style{Open: "#"}
	styleSlash = Style{Open: "//"}
	styleSemi  = Style{Open: ";"}
	styleDash  = Style{Open: "--"}
)

var styleByExt = map[string]Style{
	".md":         styleHTML,
	".markdown":   styleHTML,
	".mdx":        styleHTML,
	".mdc":        styleHTML,
	".html":       styleHTML,
	".xml":        styleHTML,
	".yaml":       styleHash,
	".yml":        styleHash,
	".toml":       styleHash,
	".sh":         styleHash,
	".bash":       styleHash,
	".zsh":        styleHash,
	".fish":       styleHash,
	".py":         styleHash,
	".rb":         styleHash,
	".pl":         styleHash,
	".r":          styleHash,
	".mk":         styleHash,
	".dockerfile": styleHash,
	".gitignore":  styleHash,
	".go":         styleSlash,
	".js":         styleSlash,
	".jsx":        styleSlash,
	".ts":         styleSlash,
	".tsx":        styleSlash,
	".java":       styleSlash,
	".c":          styleSlash,
	".h":          styleSlash,
	".cpp":        styleSlash,
	".rs":         styleSlash,
	".swift":      styleSlash,
	".kt":         styleSlash,
	".scala":      styleSlash,
	".php":        styleSlash,
	".ini":        styleSemi,
	".el":         styleSemi,
	".sql":        styleDash,
	".lua":        styleDash,
	".hs":         styleDash,
}

// Filenames that carry no extension but have a known comment style.
var styleByBase = map[string]Style{
	"Makefile":       styleHash,
	"Dockerfile":     styleHash,
	".gitignore":     styleHash,
	".gitattributes": styleHash,
	".dockerignore":  styleHash,
	".editorconfig":  styleHash,
	".env":           styleHash,
}

// StyleFor returns the comment style ilk uses for markers in path. Unknown file
// types fall back to `#`, which is by far the most common line-comment syntax in
// the config files ilk tends to touch.
func StyleFor(path string) Style {
	base := filepath.Base(path)
	if s, ok := styleByBase[base]; ok {
		return s
	}
	if s, ok := styleByExt[strings.ToLower(filepath.Ext(base))]; ok {
		return s
	}
	return styleHash
}

// Marker identifies one ilk-owned region. A file may hold many regions, but at
// most one per (layer, region) pair.
type Marker struct {
	Layer  string
	Region string
}

func (m Marker) begin() string {
	return fmt.Sprintf("ilk:begin layer=%s region=%s", m.Layer, m.Region)
}

func (m Marker) end() string {
	return fmt.Sprintf("ilk:end layer=%s region=%s", m.Layer, m.Region)
}

// Warning is emitted on the ilk-owned marker lines so a human who opens the file
// knows not to hand-edit inside them.
const Warning = "managed by ilk — edits inside this block are overwritten; run `ilk rm` to remove it"

type bounds struct {
	begin, end int // line indices; end is the index of the closing marker line
}

// locate finds the region's marker lines in lines.
func locate(lines []string, style Style, m Marker) (bounds, bool, error) {
	beginTok, endTok := m.begin(), m.end()
	b := bounds{begin: -1, end: -1}
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if !strings.HasPrefix(t, style.Open) {
			continue
		}
		switch {
		case strings.Contains(t, beginTok):
			if b.begin >= 0 {
				return b, false, fmt.Errorf("duplicate ilk:begin for layer=%s region=%s at lines %d and %d", m.Layer, m.Region, b.begin+1, i+1)
			}
			b.begin = i
		case strings.Contains(t, endTok):
			if b.begin < 0 {
				return b, false, fmt.Errorf("ilk:end without ilk:begin for layer=%s region=%s at line %d", m.Layer, m.Region, i+1)
			}
			b.end = i
			return b, true, nil
		}
	}
	if b.begin >= 0 {
		return b, false, fmt.Errorf("unterminated ilk region layer=%s region=%s opened at line %d: add the closing marker or delete the block", m.Layer, m.Region, b.begin+1)
	}
	return b, false, nil
}

func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

// join reassembles lines, preserving the convention that a non-empty file ends
// with exactly one trailing newline.
func join(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// Upsert writes body into the region identified by m. If the region already
// exists its contents are replaced in place; otherwise the region is appended to
// the end of content. Everything outside the markers is preserved byte for byte.
func Upsert(content string, style Style, m Marker, body string) (string, error) {
	lines := splitLines(content)
	b, found, err := locate(lines, style, m)
	if err != nil {
		return "", err
	}

	block := []string{style.wrap(m.begin() + " — " + Warning)}
	block = append(block, splitLines(body)...)
	block = append(block, style.wrap(m.end()))

	if found {
		out := make([]string, 0, len(lines)-(b.end-b.begin)+len(block))
		out = append(out, lines[:b.begin]...)
		out = append(out, block...)
		out = append(out, lines[b.end+1:]...)
		return join(out), nil
	}

	out := make([]string, 0, len(lines)+len(block)+1)
	out = append(out, lines...)
	if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
		out = append(out, "")
	}
	out = append(out, block...)
	return join(out), nil
}

// Extract returns the body of the region identified by m.
func Extract(content string, style Style, m Marker) (string, bool, error) {
	lines := splitLines(content)
	b, found, err := locate(lines, style, m)
	if err != nil || !found {
		return "", false, err
	}
	return join(lines[b.begin+1 : b.end]), true, nil
}

// Remove deletes the region identified by m along with its markers. It also
// collapses the blank line that Upsert inserted before an appended region, so
// that add-then-rm is a byte-for-byte round trip.
func Remove(content string, style Style, m Marker) (string, bool, error) {
	lines := splitLines(content)
	b, found, err := locate(lines, style, m)
	if err != nil || !found {
		return content, false, err
	}

	head := lines[:b.begin]
	tail := lines[b.end+1:]

	// Collapse the separator blank line Upsert inserted, so that removing several
	// regions in turn does not leave a growing gap where they used to be.
	if len(tail) == 0 {
		// The region ran to the end of the file: drop every trailing blank.
		for len(head) > 0 && strings.TrimSpace(head[len(head)-1]) == "" {
			head = head[:len(head)-1]
		}
	} else if len(head) > 0 && strings.TrimSpace(head[len(head)-1]) == "" && strings.TrimSpace(tail[0]) == "" {
		// The region sat between two blank lines: keep one, not two.
		head = head[:len(head)-1]
	}

	out := make([]string, 0, len(head)+len(tail))
	out = append(out, head...)
	out = append(out, tail...)
	return join(out), true, nil
}

// Has reports whether content carries the region identified by m.
func Has(content string, style Style, m Marker) bool {
	_, found, err := locate(splitLines(content), style, m)
	return err == nil && found
}

// Markers returns every ilk marker present in content, in file order. It is used
// to detect regions left behind by layers that are no longer adopted.
func Markers(content string, style Style) []Marker {
	var found []Marker
	for _, ln := range splitLines(content) {
		t := strings.TrimSpace(ln)
		if !strings.HasPrefix(t, style.Open) {
			continue
		}
		idx := strings.Index(t, "ilk:begin ")
		if idx < 0 {
			continue
		}
		rest := t[idx+len("ilk:begin "):]
		var m Marker
		for _, field := range strings.Fields(rest) {
			k, v, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			switch k {
			case "layer":
				m.Layer = v
			case "region":
				m.Region = v
			}
		}
		if m.Layer != "" && m.Region != "" {
			found = append(found, m)
		}
	}
	return found
}

// AppendOnce appends body to content exactly once, keyed by m. Re-running it is
// a no-op, and Remove strips it cleanly. It is implemented as a region so that
// append-once and region modes share one provenance mechanism.
func AppendOnce(content string, style Style, m Marker, body string) (string, error) {
	if Has(content, style, m) {
		return content, nil
	}
	return Upsert(content, style, m, body)
}
