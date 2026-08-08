# Preview an infrastructure change

A preview is the only honest answer to "what does this change do". The program is
what you think it does; the diff against real state is what it actually does, and
the two disagree more often for infrastructure than for anything else — because
half the inputs are not in the repository.

## When

Editing anything under the infrastructure group, before proposing it. When
`ilk check` reports `infra.preview`. When somebody asks whether a change is safe.

## Procedure

1. **Work out which stacks you touched.** One directory is one stack, with its own
   state and its own blast radius. A change to `infra/dns` cannot affect
   `infra/auth`, and previewing both wastes minutes for no information.

2. **Run the gate.** It previews exactly the stacks the change touched:

   ```
   ilk check --only infra.preview
   ```

   To iterate faster, run the repository's `infra.preview.command` yourself from
   inside the stack directory. `ilk info pulumi` prints it.

3. **Read the diff, resource by resource.** Not the summary line. The counts are
   the least interesting part of the output, and the one people quote.

   - A `replace` is a delete followed by a create. For a database, a bucket or
     anything holding state, that is data loss dressed up as an update. Find which
     property forces it and say so.
   - A `delete` you did not intend is usually a resource that moved between stacks
     or was renamed. Say which.
   - An unexpected `create` usually means the stack you previewed is not the stack
     you meant.

4. **Write down what it will do**, in resources rather than in adjectives. "Adds
   one CNAME and replaces the certificate, which reissues it and drops the current
   one for about thirty seconds" is a handoff. "Should be fine" is not.

5. **Stop.** A clean preview makes the change ready for a person to apply. It does
   not make it applied, and applying it is not your call — see
   `hand-off-an-infrastructure-apply`.

## When the preview will not run

- **`infra.preview` skipped for want of `PULUMI_ACCESS_TOKEN`.** There are no
  credentials here, so the check declined to guess rather than reporting an auth
  failure as a broken repository. A skipped check is not a passing one — say the
  preview did not run, and do not report the change as verified.
- **The command exits complaining about a stack.** `pulumi` needs to know which
  stack, and the working directory does not tell it. That belongs in
  `infra.preview.command` (`--stack prod`), not in a one-off invocation nobody
  else will repeat.
- **It hangs.** Something is prompting. Add `--non-interactive`.

## What not to do

- **Do not run the apply command to find out what a preview would have said.**
  This is the one failure this layer exists to prevent, and it is a plausible
  mistake rather than a careless one — the apply prints a diff too, and asks
  before proceeding. In a non-interactive session it does not ask.
- **Do not paste config values into a stack file to make a preview run.** A
  preview that only works because a credential is now in the repository has cost
  more than it found. Secrets go in with `pulumi config set --secret`.
- **Do not treat a refresh diff as your change.** Drift that was already there is
  worth reporting, and it is somebody else's change.
