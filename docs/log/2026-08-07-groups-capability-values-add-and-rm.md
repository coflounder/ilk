---
date: 2026-08-07
title: Groups, capability values, add and rm
---

# Groups, capability values, add and rm

The pass before ilk gets its first external consumer, so everything here is a breaking
change made while breaking things is still free.

## The root was growing one folder per layer

Nine top-level entries, eight of them from layers, none saying how they relate. Layers
now declare a directory as `group:` plus `name:`, resolving to `docs/plans`,
`docs/log`, `docs/reference` and so on. Each group gets a `README.md` with a fenced index
listing its children in declared order, with the purpose each layer stated. This
repository's root went from nine entries to six.

Ordering deliberately does *not* appear in paths — the reasoning is in
[DEC-groups-not-ordinals](../reference/DEC-groups-not-ordinals.md), and the short version
is that a shared integer namespace is not something independently published layers can
coordinate.

`scratch/` stays at the root. The ungoverned annex must not look governed.

## The change that made it possible

Six layers redeclared a peer's directory:

```yaml
plans_dir:
  default: plans
  description: … Must match the record layer's plans directory.
```

`requires: [record.plans]` bought the capability but not the path, so every consumer
carried a duplicate literal and moving the directory would have broken all of them
without a single check failing. `provides:` now takes a value:

```yaml
provides:
  record.plans: "{{ .Vars.group }}/{{ .Vars.plans_dir }}"
```

and consumers read `{{ index .Caps "record.plans" }}`. The list form still parses, since
most capabilities have nothing to carry. A value renders against `.Vars` and `.Repo`
only — it cannot read `.Caps`, because capability resolution is what is being computed.

## adopt/drop became add/rm

Hard rename, no aliases. Also `.claude/skills/*/SKILL.md` copies became per-skill
symlinks into `.agents/skills/` — per-skill rather than one link for the directory,
because `.claude/skills/` is a shared namespace and replacing it wholesale would displace
hand-written skills ilk did not put there. That needed a target-only `ModeSymlink`, which
plans, applies and prunes without ever reading through the link.

`.agent/skills/` also became `.agents/skills/`, matching the path
[npx skills](https://github.com/vercel-labs/skills) uses across its supported agents. One
letter, and it decides whether ilk's skills are visible to that tooling at all.

## The bug this nearly shipped with

`.ilk/lock.json` called its list `layers`, but it holds three kinds of thing: layers,
`ilk/core`, and `target:*` adapters. Renamed to `owners` with an explicit `kind`.

The rename silently broke reading every existing lockfile. `Owners` was absent from the
old JSON, so it unmarshalled to nil, and ilk concluded it had never written anything
here — no drift detection, no cleanup, no removals planned. It surfaced as a directory
left behind during this repository's own migration, and the full test suite was green
throughout, because nothing tested reading a lockfile written by an earlier version.

`lock.Load` now falls back to the old key and derives `kind` on read.
`internal/lock/lock_test.go` covers it. Worth remembering that a schema change to a
persisted file needs a test that reads the *old* file — the new code round-tripping
cleanly with itself proves nothing about the repositories already out there.

## Upgrading an existing repository

`ilk apply` creates the new directories and removes what it owns, but it will not move a
document somebody wrote. Move them yourself:

```sh
mkdir -p docs/reference
git mv docs/*.md docs/reference/
git mv plans docs/plans
git mv log docs/log
ilk apply
```

Then fix the relative links between record documents — siblings inside a group are now
`../reference/`, not `../docs/`. `ilk check` catches the broken ones with `record.links`.

A repository that would rather keep the flat layout can set the variables:
`ilk add record --var group=. ` is not supported, but each layer's directory names remain
variables, and a layer declaring `path:` instead of `group:`/`name:` is still valid.

## Also

- `ilk layer test` stubbed every required capability with `true`, which was a valid no-op
  command and became a directory called `true/` the moment capabilities started naming
  places. Command capabilities still get `true`; the rest get a sandbox path.
- Three layers specified and not built: `pulumi` (the first `infra` group tenant, blocked
  on the same credential story as `linear-mirror`), `autoresearch` (staleness by expiry
  rather than by coupling, since the world moving is not something watching this
  repository can detect), and `brainstorm` — which argues itself out of existing, on the
  grounds that a check counting alternatives is satisfied most cheaply by writing
  strawmen. See [layers/README.md](../../layers/README.md).

## `ilk self update`

The project's own AGENTS.md has always warned that the built-in layers are
embedded, so a stale binary silently serves stale templates. That was a warning
with no command behind it. There is one now:

```sh
ilk self update --path ../ilk    # rebuild this binary from that checkout
ilk upgrade                      # reconcile this repository to what it embeds
```

It builds the working tree as it stands, uncommitted changes included, since the
loop it serves is developing ilk rather than fetching a release.

Three things it does that a bare `go build -o` does not:

- **Refuses a source that is not ilk.** It reads `go.mod` and checks the module
  path first, because `--path ..` in the wrong shell would otherwise cheerfully
  build some other project over your ilk binary, and the failure would show up
  later as ilk behaving like a different program.
- **Stages then renames.** A running executable cannot be written to — `cp` over
  it fails with `ETXTBSY`. It builds beside the destination and renames, which is
  atomic, and which also means a failed build never leaves you without a binary.
  It runs the new binary before standing on it.
- **Compares content, not version strings.** During development the version is
  derived from the commit, so it does not move while the code does. Reporting
  "unchanged" after a rebuild that did change the binary would be a lie told at
  exactly the moment somebody is checking whether their edit landed.

It lives under `self` rather than at the top level, and that was the second
decision. `ilk update` beside `ilk upgrade` differ by one letter and by which of
two very different things they touch — the binary, or a repository's layers. The
alternatives considered were renaming the layer command to `ilk refresh` or `ilk
pull`. Both were rejected:

- `pull` promises a fetch that mostly does not happen. `ilk init` gives you
  `toolkit`, `record` and `gates`, all embedded in the binary; there is no remote
  to pull from, and local-path layers have the same problem.
- `refresh` trades one collision for another. `ilk apply` already means "reconcile
  to declared state", and "re-resolve then reconcile" is close enough to be the
  same confusion wearing a different word.

The `self` namespace is [rustup's answer](https://rust-lang.github.io/rustup/basics.html)
to exactly this, and it makes the distinction structural rather than lexical: the
object being acted on is named, so it cannot be misread. It also costs 9
references rather than the 30 across 22 files that renaming `upgrade` would have.
`ilk self version` and `ilk self uninstall` now have somewhere to live.

## quality-gates became gates

The layer id is `ilk/gates`; its directory moved to `internal/builtin/layers/gates`.
Nothing else moved, because the checks and capabilities it registers were already
`gates.tests`, `gates.lint`, `gates.build` and `gates.review` — the `quality-`
prefix only ever appeared in the layer's own name, which is a reasonable sign it
was not carrying meaning.

Two things the sweep nearly missed, both because they did not match the obvious
pattern: `.github/workflows/ci.yml` called `"$bin" drop quality-gates`, which the
earlier `adopt`/`drop` rename skipped for want of an `ilk ` prefix; and the
brownfield and staleness CI jobs still wrote fixtures to `docs/`, which is the
group now rather than a governed directory, so the baseline assertions would have
failed. All four CI jobs were run locally against the built binary rather than
assumed.
