# 4. Mutation testing with go-mutesting, on a copy, on changed code

## Decision

`rumblestrip mutate` runs go-mutesting on a copy of the module, in the packages a change touches. The copy holds the files git sees, tracked or new, so build outputs aren't copied.

- A mutant counts only if it starts on a line the change adds, so old code's survivors never fail a new change.
- A mutant that only removes the return of an `if err != nil` block isn't counted, and the report says how many there were.
- The gate fails under `--min`, 0.8, which the template calibrates (`docs/rules.md`, M6). It's a score rather than zero survivors, because some mutants change nothing a test could see.

## Why

- go-mutesting removes statements, branches and expressions, and runs on Windows.
- go-mutesting writes each mutant over the source file while the tests run, and rapid saves its failing inputs under `testdata/`. Running it on the real tree would leave both behind, so it runs on a copy.
- In Go, every `if err != nil { return err }` is a branch that lives unless a test makes the call fail. Failing a disk or system call in a test rarely pays off. Meanwhile, the compiler refuses an unused `err`, and errcheck and ineffassign refuse a dropped one.

## Rejected

gremlins has `--diff` and thresholds, but it only swaps operators, and on Windows it reports every mutant as not covered.

## Consequences

- A mutant counts on the line where it starts. Removing a block whose first line the change left alone isn't counted.
- An error's return goes untested by mutation. A refusal, such as `errors.Is(err, fs.ErrExist)` or a sentinel error, is behaviour, so it stays counted.
- Each mutant means a recompile plus the package's tests. That's why the gate runs in the merge check, not in the agent's stop hook.
