# 8. What the owner reads on a pull request

## Decision

- Each gate of `rumblestrip check` runs in a CI job of its own, and all of them run at once. Each job is named for the owner. `Check`, the job the ruleset requires, passes once every gate has.
- The owner's approval is a commit status, `Rumblestrip / Owner approval`. The review sheet's job posts it, and the ruleset requires it. No gate fails for want of it.
- Rumblestrip's comment opens with a status line, which each job rewrites as the pull request moves on.
- The run has one summary, Check's. It lists every check and what it checks, then what each failing gate wants and what it found. Each finding with a line is also an annotation on that line.
- The merge job writes the commit it lands. Its message is the pull request's title, a conventional commit of 72 characters at most, plus its description. The App's bot is the author, and the owner the co-author.

## Why

- A single job that fails doesn't say whose move it is: a failed test and a spec awaiting approval look the same. A log is written for the agent, not the owner.
- A check that fails for want of approval reads as broken, while a pending status reads as waiting.
- GitHub's checks list shows a commit status's description, but no text from a workflow. Rumblestrip's comment sits on the pull request's page.
- The approval status counts only when it comes from GitHub Actions, and from the one job allowed to write statuses, which runs none of the change's code. Code under test could read a job's token.
- GitHub counts a skipped check as passed, so `Check` never skips. The condition that skips an edit of the description alone sits on the caller's job.
- When GitHub squashes, it adds every author of the branch's commits as a co-author, the bot included, and keeps the commits' trailers. That's why the merge job writes the message itself.

## Consequences

Each gate's job installs only its own tool. A run is about twenty jobs, and GitHub's free plan runs at most twenty at once, so two runs in a stack queue.
