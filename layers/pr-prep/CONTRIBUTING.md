What this layer wants from a proposal:

  - Show the description you actually opened the request with, next to what
    `ilk pr-prep draft` produced. The gap between the two is the layer's real
    specification, and nobody but an adopter can see it.
  - If you edited the draft the same way twice, that edit belongs in the command.
    A section this layer does not know how to read is the most useful thing you can
    report, because it looks like nothing is wrong — the draft is simply thinner
    than it should be.
  - If `prprep.evidence-checkable` rejected evidence that was genuinely checkable,
    quote the bullet. The heuristic is deliberately generous and a false positive
    means it is not generous enough in a way this repository has not seen.
  - Say whether you stopped using the command, and at what point. A layer whose
    output people quietly abandon reports as healthy, which is the failure it is
    least able to detect on its own.
