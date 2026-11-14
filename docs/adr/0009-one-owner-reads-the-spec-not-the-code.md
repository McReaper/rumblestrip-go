# 9. One owner, who reads the spec and not the code

## Decision

- Rumblestrip is for a project with one owner and an AI agent that writes the code. The owner approves the spec, the harness, the dependencies and the Go line. They never review the code.
- Every check exists so the owner doesn't have to read code. A change to Rumblestrip that adds code to what the owner reads goes against this decision. When a check misses something, the fix is another check, not asking the owner to read.
- There is no team mode for now, and no setting that requires code review.

## Why

- Review catches fewer bugs than people expect. In Microsoft's study of code review (Bacchelli and Bird, 2013), finding defects was the first reason developers gave for reviewing, yet what reviews delivered was mostly shared knowledge and awareness. An often-cited Cisco study found reviewers catch far fewer defects past about 400 lines at a time, and an agent's pull request often runs past that.
- Machines don't tire. The checks read every line, every time, against the same rules.
- Only the owner can judge intent: whether the app should do something. The spec says it in a few lines per feature, where the code takes hundreds.
- Rumblestrip's first version had the owner read the tests, the code, a summary and the harness on every pull request. The reading was the bottleneck, and the owner judged it a failure.

## What it gives up

- In a team, review is also how people learn the code. When nobody reads it, whoever debugs an incident reads it for the first time.
- Some organisations require a second person to approve every change. Rumblestrip doesn't provide that.
- Rules judge how the code is organised: architecture, size, duplication. They don't catch one idea written twice under two names. That is left to an AI scan of the code, outside the pull request.

## Later

A team mode may come, likely with the .NET version. It would offer two levels:
- the spec, plus code review of what is new (a module, a public command), helped by AI;
- for regulated work, a person approving every change after Rumblestrip has done the mechanical checks.
