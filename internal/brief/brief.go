// Package brief assembles the session-start packet: what an agent needs to know
// before it does anything, computed from the repository rather than typed by
// whoever opened the session.
//
// This is the mechanism behind "no session starts junior". A new agent, an agent
// resuming after a pause, and a replacement for one that died all get the same
// orientation, and none of it depends on someone remembering the right prompt.
package brief

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/coflounder/ilk/internal/checks"
	"github.com/coflounder/ilk/internal/engine"
)

// Brief is the assembled packet.
type Brief struct {
	Project      string       `json:"project"`
	IlkVersion   string       `json:"ilk_version"`
	Layers       []LayerInfo  `json:"layers"`
	Contract     []DirInfo    `json:"contract"`
	Recent       []FileInfo   `json:"recent,omitempty"`
	Skills       []SkillInfo  `json:"skills,omitempty"`
	Commands     []CommandRef `json:"commands,omitempty"`
	Capabilities []Capability `json:"capabilities,omitempty"`
	Checks       *CheckStatus `json:"checks,omitempty"`
	Warnings     []string     `json:"warnings,omitempty"`
}

// LayerInfo is an adopted layer.
type LayerInfo struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Summary string `json:"summary"`
}

// DirInfo is one directory in the repository's contract.
type DirInfo struct {
	Path    string `json:"path"`
	Purpose string `json:"purpose,omitempty"`
	Files   int    `json:"files"`
}

// FileInfo is a recently touched record file.
type FileInfo struct {
	Path    string `json:"path"`
	Title   string `json:"title,omitempty"`
	Status  string `json:"status,omitempty"`
	Updated string `json:"updated,omitempty"`
}

// SkillInfo is an on-demand procedure available in this repository.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

// CommandRef is a command an agent can run.
type CommandRef struct {
	Command string `json:"command"`
	Summary string `json:"summary"`
}

// Capability is a verification command the repository supplies.
type Capability struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CheckStatus summarises validation without making the caller run it again.
type CheckStatus struct {
	Passed  int             `json:"passed"`
	Failed  int             `json:"failed"`
	Skipped int             `json:"skipped"`
	Errored int             `json:"errored"`
	Failing []checks.Result `json:"failing,omitempty"`
	Partial bool            `json:"partial"`
}

// Options tunes assembly.
type Options struct {
	// Full runs command-based checks too. Off by default, because a brief that
	// takes two minutes to print is a brief nobody wires into session start.
	Full bool
	// RecentLimit caps the recently-changed listing.
	RecentLimit int
}

// Build assembles the packet.
func Build(p *engine.Project, opts Options) (*Brief, error) {
	if opts.RecentLimit == 0 {
		opts.RecentLimit = 5
	}

	b := &Brief{
		Project:    p.Repo.Name(),
		IlkVersion: p.Version,
	}

	for _, l := range p.Layers {
		b.Layers = append(b.Layers, LayerInfo{
			ID:      l.ID(),
			Version: l.Loaded.Manifest.Version,
			Summary: l.Loaded.Manifest.Summary,
		})
	}

	desired, err := p.Desired()
	if err != nil {
		return nil, err
	}
	seenDir := map[string]bool{}
	for _, d := range desired {
		if !d.Dir || seenDir[d.Path] {
			continue
		}
		seenDir[d.Path] = true
		b.Contract = append(b.Contract, DirInfo{
			Path:    d.Path,
			Purpose: d.Purpose,
			Files:   countMarkdown(p.Repo.Path(d.Path)),
		})
	}
	sort.Slice(b.Contract, func(i, j int) bool { return b.Contract[i].Path < b.Contract[j].Path })

	b.Recent = recentFiles(p, b.Contract, opts.RecentLimit)

	in, err := p.TargetInput()
	if err != nil {
		return nil, err
	}
	for _, s := range in.AllSkills() {
		b.Skills = append(b.Skills, SkillInfo{
			Name:        s.Name,
			Description: s.Description,
			Path:        fmt.Sprintf(".agent/skills/%s/SKILL.md", s.Name),
		})
	}

	b.Commands = []CommandRef{
		{Command: "ilk check", Summary: "Validate the repository. Every failure prints its own fix."},
		{Command: "ilk brief", Summary: "Print this packet again, after the state has moved on."},
		{Command: "ilk status", Summary: "Show adopted layers and anything that has drifted."},
	}
	for _, l := range p.Layers {
		for _, c := range l.Loaded.Manifest.Commands {
			b.Commands = append(b.Commands, CommandRef{
				Command: fmt.Sprintf("ilk %s %s", l.Name(), c.Name),
				Summary: c.Summary,
			})
		}
	}

	for name, value := range p.Config.Capabilities {
		b.Capabilities = append(b.Capabilities, Capability{Name: name, Value: value})
	}
	sort.Slice(b.Capabilities, func(i, j int) bool { return b.Capabilities[i].Name < b.Capabilities[j].Name })

	report, err := checks.Run(p, checks.Options{Timeout: 30 * time.Second})
	if err == nil {
		status := &CheckStatus{
			Passed:  report.Passed,
			Failed:  report.Failed,
			Skipped: report.Skipped,
			Errored: report.Errored,
			Partial: !opts.Full,
		}
		for _, r := range report.Results {
			if r.Status == checks.StatusFail || r.Status == checks.StatusError {
				status.Failing = append(status.Failing, r)
			}
		}
		b.Checks = status
	} else {
		b.Warnings = append(b.Warnings, "checks could not run: "+err.Error())
	}

	for id, missing := range p.MissingRequirements() {
		b.Warnings = append(b.Warnings, fmt.Sprintf("%s requires %s, which nothing supplies", id, strings.Join(missing, ", ")))
	}
	sort.Strings(b.Warnings)

	return b, nil
}

func countMarkdown(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".md") && e.Name() != "README.md" {
			n++
		}
	}
	return n
}

// recentFiles lists the most recently modified record documents, which is the
// cheapest useful answer to "what has been happening here".
func recentFiles(p *engine.Project, contract []DirInfo, limit int) []FileInfo {
	type candidate struct {
		path string
		mod  time.Time
	}
	var candidates []candidate
	for _, d := range contract {
		entries, err := os.ReadDir(p.Repo.Path(d.Path))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") || e.Name() == "README.md" {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			candidates = append(candidates, candidate{filepath.Join(d.Path, e.Name()), info.ModTime()})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].mod.After(candidates[j].mod) })
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	var out []FileInfo
	for _, c := range candidates {
		fi := FileInfo{Path: c.path}
		if data, err := os.ReadFile(p.Repo.Path(c.path)); err == nil {
			meta := frontmatterOf(string(data))
			fi.Title = meta["title"]
			fi.Status = meta["status"]
			fi.Updated = meta["updated"]
		}
		out = append(out, fi)
	}
	return out
}

// frontmatterOf reads the handful of scalar keys the brief displays.
func frontmatterOf(content string) map[string]string {
	out := map[string]string{}
	if !strings.HasPrefix(content, "---") {
		return out
	}
	lines := strings.Split(content, "\n")
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return out
}
