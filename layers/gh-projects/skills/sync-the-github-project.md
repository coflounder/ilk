---
name: sync-the-github-project
description: Reconcile the plan with a GitHub Project, or adopt a board that already has items. Use when the board and the record disagree, when a new spec needs to appear on the board, or when setting this up for the first time.
---

# Sync the GitHub Project

The documents in `{{ .Vars.plans_dir }}/` are the source of truth. The GitHub Project
is a view of them. Everything below follows from that: the board is written to, and
never read back.

## The two commands

```
ilk mirror plan gh-projects     # what would change on the board
ilk mirror apply gh-projects    # make the board match
```

`plan` reads the board and writes nothing anywhere — not to GitHub, not to the
documents. Run it freely. `apply` shows the same plan and then asks before writing;
`--yes` skips the question for a hook or a CI job.

## What the plan means

| marker | meaning |
|---|---|
| `+ create` | a document with no board item yet — one will be created |
| `~ update` | the board's title or status disagrees with the document |
| `= link` | an existing board item matched to a document by title |
| `? orphan` | the document points at an item that is no longer on the board |
| `· untracked` | a board item no document claims |
| `! AMBIGUOUS` | the title could mean more than one item — refused |

`orphan` and `untracked` are reported, never acted on. ilk does not delete from the
board and does not recreate an item somebody deliberately removed. Deciding an item
is dead is a person's call.

## First run against a board that already has items

Do not let `apply` create duplicates of items that already exist. Link first:

```
ilk mirror link gh-projects
```

This matches unlinked documents to existing items by title and records the match in
each document's `github:` frontmatter key. From then on identity is exact and the
titles are free to diverge.

Where one title could mean two items, `link` refuses and names both. Resolve it by
either renaming one side so the titles differ, or writing the right id into the
document by hand:

```yaml
github:
  id: PVTI_lADOA...
  url: https://github.com/acme/api/issues/214
```

Never guess. A wrong link is silent: every later sync writes to the wrong item, and
nobody notices until somebody reads the board and does not recognise it.

## Status

A document's `status:` is written to the project's `{{ .Vars.status_field }}` field.
The value must be one the field already offers — matching is case-insensitive, so
`in progress` finds `In Progress`. If it offers no such option the sync refuses and
lists what it does offer. Add the option to the board, or change the document; do
not expect ilk to invent one.

## Configuration

Set these for this layer in `.ilk/config.yaml` (`ilk info gh-projects` lists them all):

- `owner` — the user or organisation the project belongs to. Required.
- `project_number` — the number in the project's URL. Required.
- `repo` — set it to `owner/name` and each new item is a real issue in that
  repository, referenceable from commits and pull requests. Leave it empty and items
  are drafts on the board, which need no repository but cannot be referenced.
- `status_field` — the single-select field holding workflow state. Defaults to
  `Status`.

`ilk check --only gh.configured` verifies the whole path: `gh` installed, `gh`
authenticated, the project readable. Run it before the first sync.

## When something fails

A provider failure stops that one document, not the run. `apply` reports which
documents failed and why, and the rest still go through — re-running after the fix
picks up exactly what is left.

If `gh` reports a permission problem on the project specifically, the token is
missing the `project` scope:

```
gh auth refresh -s project
```
