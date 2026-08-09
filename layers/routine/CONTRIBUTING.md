What this layer wants from a proposal:

  - Say what the routine replaced. The interesting cases are the ones where a
    schedule was the wrong answer and a check, a hook or a gate turned out to be the
    right one — that is a change to the skill, and it is worth more than another
    field in the frontmatter.
  - If a routine hit its budget, say what it was doing when it was killed. A budget
    that is always hit and a budget that is hit once a quarter want opposite fixes.
  - If you had to edit the generated runner file by hand, say exactly what you added
    and why the projection could not produce it. That is the strongest argument
    there is for changing what `sync` writes, and the only one that is not a guess.
  - Do not send a new `runner:` target without a repository that runs on it. A
    projection nobody exercises decays silently, which is the failure this layer is
    least able to notice.
