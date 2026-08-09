# Hand off an infrastructure apply

Applying infrastructure is human work. Not because an agent would run the wrong
command, but because the command is right and the consequence is unrecoverable:
a `replace` on a database is a delete and a create, and there is no branch to
revert it on.

So this layer declares `infra.up.command` and then wires it to nothing. No check
runs it, no hook runs it, no subcommand runs it. If you find yourself reaching for
it, the thing to produce is this handoff instead.

## When

A preview is clean and the change now needs applying. Somebody says deploy, apply,
roll out, ship it, or push it to prod. Somebody asks you to run `pulumi up`.

## Procedure

1. **Confirm the preview is clean and current.** Not from memory — from a run since
   the last edit. `ilk check --only infra.preview`.

2. **Write the handoff.** It is short, and every line is something the person
   applying it cannot reconstruct in ten seconds:

   - The stack, as a directory: `infra/dns`.
   - The exact command to run, with the working directory it runs in.
   - What the preview said, in resources: what is created, updated, replaced,
     deleted. Name every replace and every delete individually.
   - What breaks while it runs, and for how long.
   - How to undo it, or that it cannot be undone. "Cannot be undone" is a
     perfectly good answer and by far the most useful one.

3. **Give them the command rather than running it.** The apply command for this
   repository is `infra.up.command`; `ilk info pulumi` prints its value. Present
   it as text for a person to run.

4. **Say what you will do afterwards** — re-run the preview to confirm it is
   empty, check the record needs updating — and then wait.

## Example

```
Ready to apply: infra/dns

  cd infra/dns && pulumi up --stack prod

Preview: 1 create, 1 replace, 0 delete.
  + aws:route53:Record  api-cname
  ± aws:acm:Certificate  api-cert   (replaced: domainName changed)

The certificate replace reissues and validates a new cert before the old one is
released; expect roughly 30 seconds where api.example.com serves the old cert.

Not reversible by re-running: the old certificate is deleted on success.
```

## What not to do

- **Do not run the apply because the preview was boring.** A no-op preview is the
  case where running it seems safest and buys the least.
- **Do not run it because you were told to.** Being asked to deploy is the
  situation this skill is for, not an exception to it. Produce the handoff and say
  why. If the person wants it run anyway, they now have the command.
- **Do not narrow the handoff to the summary counts.** "3 changes" is what the
  person could have read themselves; which three is the work.
- **Do not treat a scheduled or CI-driven apply as the same thing.** A pipeline
  that applies on merge is a policy somebody decided and can audit. An agent
  applying inside a session is neither.
