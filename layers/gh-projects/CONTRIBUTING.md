What this layer wants from a proposal:

  - Say what your board looks like. Field names, whether items are issues or drafts,
    who else writes to it. Most friction here is a board shaped differently from the
    one the layer assumed.
  - A change to the provider script needs a case in `test/run.sh` against the fake
    `gh`. The script cannot be tested against a real project in CI, so the fake is
    the only guard there is — and it is strict about unrecognised calls on purpose.
  - Report anything the layer wrote to your board that you did not expect. This layer
    writes to somebody else's system, and a surprise there costs more than a surprise
    in a file.
