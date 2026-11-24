# 7. A spec and its build are a stack, and Rumblestrip merges them

## Decision

The build's pull request sits on the spec's branch, and the two form one GitHub stack.

Rumblestrip itself merges a pull request into the default branch once its check passes:

- through GitHub's asynchronous merge;
- as a GitHub App, which needs contents and pull requests write. It can be the agent's App or any other. `rumblestrip init github` stores its Client ID and key as the repository's secrets.

It turns on no auto-merge, and no job of its own moves pull requests or deletes branches.

## Why

- **Stacks do the moving.** Once a pull request merges, GitHub moves the next one in its stack onto the default branch: it rebases it, retargets it, and runs its check again. A pull request outside a stack would stay on a merged branch. That's why the check fails a pull request on another branch that isn't in a stack.
- **Auto-merge can't merge a stack.** It refuses a stack's pull request through the API ("Auto-merge is not supported for stacked pull requests"). A pull request with auto-merge on can't join a stack either. The asynchronous merge is the one GitHub names for stacks, and the ruleset still decides when it runs.
- **The merge must come from an App, not the workflow's own token.**
  - After a merge by the workflow's token, GitHub deleted no branch. It moved the next pull request onto the default branch without rebasing it or running its check, so that pull request sat in conflict.
  - Events of that token also start no workflow.
- **The approval re-runs the failed run.** A check run apart on approval would leave the failed run on the commit, and GitHub would hold the merge on that run. So the owner's approval re-runs the run that failed for want of it, and that run's merge job merges.

## Rejected

An hourly job that moved stacked pull requests and deleted their bases did by hand what stacks do.
