What this layer wants from a proposal:

  - Say what the wireframe caught. The whole claim is that disagreement is cheaper
    before the build than after it, and every case where a sketch changed a decision —
    or failed to, and the build had to change instead — is evidence about the claim
    rather than about the tooling.
  - If the scaffold's fidelity was wrong for your work, say which way. Too plain and
    nobody can tell what they are looking at; too finished and the reactions arrive
    about the styling. That balance is the only genuinely hard thing here.
  - If `wireframe.self-contained` fired on something legitimate, send the file. The
    check is deliberately crude, and a real wireframe it refuses is a bug in the check.
  - Do not send a component library, a token set or a screenshot pipeline. A wireframe
    that is expensive to throw away stops being one, and `visual-qa` already owns what
    the built thing looks like.
