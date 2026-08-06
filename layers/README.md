# Layers

Layers that ship alongside ilk but not inside it. Adopt one with:

```sh
ilk adopt gh:coflounder/ilk/layers/blueprint
```

Pin a version with `@v0.2.0`. Nothing here is adopted by default — `ilk init` gives
you the built-in layers only, so a fresh repository still needs no network.

## What is here

Each of these takes one idea from the MetaHarness essay on
[building an AI-native SDLC](https://www.tenex.co/blog/building-an-ai-native-sdlc) and
reduces it to something that works on plain markdown and git.

| Layer | Principle | What it enforces |
|---|---|---|
| [`blueprint`](blueprint/) | *Plans are detailed, and allowed to change* | Every spec belongs to an epic and a milestone that exist; every spec says what "done" means; every epic states an outcome |
| [`compound-lessons`](compound-lessons/) | *Lessons compound for everyone* | Every lesson names the durable change it produced — a check, a skill, a convention — so "we will be more careful" cannot pass as an outcome |
| [`archive`](archive/) | *Context as code* | Superseded documents move to an archive rather than being deleted, and nothing live may cite them |

The principles ilk already covers in its built-in layers — the project record, the
session brief, verification an agent cannot talk past — are not repeated here.

## These layers run code

`blueprint` and `archive` ship shell commands (`ilk blueprint next`, `ilk archive it`),
so adopting them requires consent:

```sh
ilk info gh:coflounder/ilk/layers/blueprint     # read it first
ilk adopt gh:coflounder/ilk/layers/blueprint --allow-exec
```

That prompt is the design working, not an obstacle. `compound-lessons` is entirely
declarative and needs no flag.

## Adding one

Publishing a layer should cost about as much as publishing the post that described the
idea. There is no server and no account:

1. Write it — [docs/REF-writing-layers.md](../docs/REF-writing-layers.md) is the guide,
   and `ilk layer new` scaffolds one.
2. `ilk layer test ./your-layer` must pass. It adopts the layer into a throwaway
   repository, applies twice to prove idempotency, drops it, and asserts the repository
   came back. A layer that cannot be cleanly removed is one nobody can safely try.
3. Add an entry to [`internal/registry/registry.yaml`](../internal/registry/registry.yaml)
   so `ilk search` can find it, and open a pull request.

Your layer does not have to live in this repository. `ilk adopt gh:you/your-layer`
works whether or not it is indexed; the index only makes it discoverable.

CI runs `ilk layer validate` and `ilk layer test` against everything in this directory
on every push, so these are held to the same standard as any published layer.
