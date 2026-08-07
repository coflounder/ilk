package contrib

import (
	"fmt"
	"regexp"
	"strings"
)

// screen looks for reasons this proposal should not leave the repository.
//
// Contributing upstream publishes a diff taken from a working repository, and the
// failure mode is obvious once stated: a credential that was pasted into a managed
// file to make something work locally, now in a pull request, now in a public git
// history for ever. Rotating a leaked token is somebody's afternoon; not
// publishing it is one regex.
//
// Two severities, because they are genuinely different problems. A secret is not a
// matter of opinion and blocks submission outright. A repository's own name or an
// absolute path is often exactly what a proposal ought to say — the context is the
// evidence — so it is raised and not enforced.
//
// Nothing is ever stripped. Editing evidence on the way out would change what
// upstream is being asked to judge, and a proposal that quietly says something
// other than what the repository found is worse than no proposal.
func screen(p *Proposal) []Concern {
	var out []Concern
	for _, e := range p.Edits {
		out = append(out, screenText(e.Path, e.Current)...)
	}
	for _, s := range p.Signals {
		out = append(out, screenText(s.Subject, s.Detail)...)
	}
	return out
}

// secretPatterns are shapes that are credentials and essentially nothing else.
//
// The list is deliberately narrow. A screen that fires on anything resembling a
// hex string trains people to pass whatever flag silences it, and then it is not
// protecting them from anything.
var secretPatterns = []struct {
	re     *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{16,}`), "a GitHub token"},
	{regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`), "a GitHub fine-grained token"},
	{regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}`), "an API secret key"},
	{regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`), "a Slack token"},
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), "an AWS access key id"},
	{regexp.MustCompile(`\blin_api_[A-Za-z0-9]{20,}`), "a Linear API key"},
	{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`), "a private key"},
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.`), "a signed token"},
}

var (
	absolutePath = regexp.MustCompile(`(^|[\s"'=(:])(/(?:home|Users|root|var/folders)/[A-Za-z0-9._-]+|[A-Za-z]:\\\\?Users)`)
	emailAddress = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
)

func screenText(where, text string) []Concern {
	var out []Concern
	for i, line := range strings.Split(text, "\n") {
		n := i + 1
		for _, p := range secretPatterns {
			if p.re.MatchString(line) {
				out = append(out, Concern{
					Path: where, Line: n, Blocking: true,
					Reason: fmt.Sprintf("this looks like %s. Remove it from the file, rotate it, and run this again — a proposal is public and git history is permanent", p.reason),
				})
			}
		}
		if absolutePath.MatchString(line) {
			out = append(out, Concern{
				Path: where, Line: n,
				Reason: "an absolute path from this machine. Upstream cannot use it, and it usually means the change is local rather than general",
			})
		}
		if emailAddress.MatchString(line) && !strings.Contains(line, "example.") {
			out = append(out, Concern{
				Path: where, Line: n,
				Reason: "an email address. Check it belongs in a public proposal",
			})
		}
	}
	return out
}
