What this layer wants from a proposal:

  - **Never attach a credential.** Not a rotated one, not a redacted one, not "the
    last four characters". Send the *shape* — `a 40-character token assigned to a
    variable ending in _KEY` — and the path it was at. A proposal that carries the
    value has repeated the mistake the layer is about.
  - If the scanner fired on something that was not a credential, that is the most
    useful proposal there is: a gate people route around is not a gate. Say what the
    line looked like and what you had to do to get past it.
  - If you are adding a pattern to the bundled scanner, say what it fires on that is
    *not* a credential, not only what it catches. The bundled list is a floor for
    repositories that have configured nothing; every pattern that costs a false
    positive spends the hook's credibility. Somewhere around the fifth new pattern,
    what you actually want is `secrets.command` pointed at gitleaks, and a proposal
    saying so is worth more than the pattern.
  - If your scanner needs a credential of its own to run, say so — that is the case
    `requires_env:` exists for, and this layer does not use it because most scanners
    read files and nothing else.
  - Proposals about the skill: say what was done instead of what it says. The skill
    exists because the obvious action — delete the value, commit, report it fixed — is
    the wrong one, so every case where an agent still took it is evidence about the
    wording and not about the reader.
