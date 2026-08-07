What this layer wants from a proposal:

  - Every repository `ilk init` touches gets this layer, so the bar is
    higher than elsewhere: a change here lands everywhere, including repositories
    nobody is watching.
  - Say what the grammar could not express. The naming and frontmatter rules exist so
    a parser and a person read a document the same way; a proposal is a claim that
    something real does not fit.
  - Staleness proposals need the `covers:` globs you used. Coupling that measured
    the wrong thing is the most valuable report this layer can get, and it is almost
    never visible from inside one repository.
