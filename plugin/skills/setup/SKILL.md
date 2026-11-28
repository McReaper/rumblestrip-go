---
name: setup
description: Sets Rumblestrip up on this machine at the version the repository pins, pulling the image its checks run in and installing rumblestrip for pr. Use when a Rumblestrip hook refuses a command or an edit and says to run rumblestrip setup, on a new machine, or once the repository pins another version.
---

# Run `rumblestrip setup`

Run `rumblestrip setup`: until it has run, the hooks refuse every command and edit but this one. It pulls Rumblestrip's image for the version the repository pins, which holds every tool its checks run at the versions CI runs, installs `rumblestrip` for `app` and `init`, which run on the machine with the user's credentials, and installs `gh stack`, GitHub's extension of `gh` that stacks pull requests. It then checks that the agent can act as the App the repository names, as its `gh` and `git` do.

When it says Docker or Go isn't installed or Docker isn't running, `gh` is too old, or the agent can't act as the App, tell the user: they fix it, then you run it again. On a machine that isn't x86-64, or in a git submodule, the checks can't run: tell the user.
