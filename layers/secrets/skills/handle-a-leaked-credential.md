# Handle a leaked credential

A scanner has flagged a secret, or you have noticed one in a diff. There is one wrong
move here and it is the obvious one: delete the value, commit, report it fixed. That
leaves the credential in the history, in every clone, in every fork, and in whatever
scraped the push — and it retires the alarm that would have got it rotated.

**Rotate first. Everything else is cleanup.**

## 1. Rotate, now

Before reading the rest of this file, before writing a commit, before telling anybody:
issue a new credential and revoke the old one at the provider.

The reasoning is short. You cannot know who has read it. A pushed secret is public
until proven otherwise, and nothing you do to the repository changes what has already
been copied. Rewriting history does not un-compromise a credential; it only makes it
harder to find out that it was compromised.

If the value has **never left this machine** — not pushed, not on a branch anybody
fetched, not in a PR, not in CI logs — you may be able to amend or drop the commit and
keep the credential. Be honest about "never left". `git log origin/HEAD..HEAD` tells you
what is unpushed; it does not tell you what a backup tool, an editor plugin or a shared
sandbox already copied. When unsure, rotate. The cost of rotating unnecessarily is
twenty minutes; the cost of the other error is somebody else's account.

Rotating means all of it:

- Issue the replacement and put it where it belongs — a secret manager, the CI
  provider's secret store, the developer's own environment. Not the repository.
- **Revoke the old one.** A rotation that leaves the old key live is not a rotation.
- Check what the old credential could reach, and look at its access logs for the window
  it was exposed. A leak is also an incident until you have looked.

## 2. Purge it from history

Only now. Use a real tool — [git-filter-repo](https://github.com/newren/git-filter-repo)
or the BFG Repo-Cleaner — rather than a hand-written `filter-branch`:

```sh
git filter-repo --replace-text replacements.txt   # value ==> REDACTED
```

Then force-push, and tell everybody with a clone to re-clone rather than merge; a stale
clone will otherwise push the old objects straight back.

Understand what a purge does not reach:

- **Forks.** They are separate repositories and keep their own objects.
- **Pull request refs.** On most hosts, `refs/pull/*` survives a force-push to branches.
  Ask the host to garbage-collect them; on GitHub that is a support request.
- **Caches and mirrors.** CI caches, artifact stores, search indexes, and anything that
  cloned in the meantime.
- **Anybody who was watching.** Public pushes are scraped continuously. Automated
  credential harvesting from public commits is measured in seconds, not hours.

## 3. Rotate again if the purge was slow

If more than a few minutes passed between the push and the revocation — or if you rotated
before the purge and the purge then took a day — rotate a second time once the history is
clean. The window between exposure and revocation is the whole risk, and the second
rotation costs almost nothing.

## Reporting it

Say what leaked, where, and what has been done. Do not include the value.

**Never paste the credential into an issue, a commit message, a PR description, a chat
channel or a test.** Those are all published, indexed and long-lived, and a report that
carries the secret has moved the leak somewhere new rather than closing it. Refer to it
by what it is and where it was — `the deploy key in config/, committed in a1b2c3d` —
and let whoever needs the value get it from the provider.

The same rule applies to the fix commit. `git log` is public; "remove leaked Stripe
key" is fine, quoting it is not.

## When it is not a credential

Scanners fire on fixtures, documentation and generated examples. That is not a bug in
the scanner, and the answer is not to loosen it:

- Add the path to the allowlist (`.ilk/secrets-allow`), one substring per line, with the
  reason beside it.
- Say in the commit message why the match is not a credential. A reviewer has to be able
  to disagree.
- Use values that are obviously invalid — the provider's own documentation example, or a
  clearly fake body. A realistic-looking fake in a fixture will be reported as a real leak
  by somebody, eventually, and they will be right to.

Never allowlist a path because rotating is inconvenient. An allowlisted secret is one
nobody will ever be warned about again.

## Preventing the next one

- Read secrets from the environment or a secret manager; commit an `.env.example` with
  empty values and keep `.env` ignored.
- A credential that reached the working tree usually got there because the alternative
  was undocumented. Fix that too, in the same change.
- If the pre-commit hook was bypassed with `--no-verify`, find out why. A gate people
  route around is not a gate, and the reason is nearly always that it fires on things
  that are not credentials.
