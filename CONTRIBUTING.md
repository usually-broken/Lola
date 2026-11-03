# Contributing

Changes come in through a pull request. Branch, change, open it, and keep it to one thing —
a fix and a refactor in the same branch is two conversations happening at once.

Commits say what changed and why it had to. `Fix the config path` is enough; a commit
message is a record for whoever reads it next.

Before sending, run:

```sh
make check
```

That's gofmt, vet, every test and a Windows build — the same thing CI runs.

Tests live beside the code they test. A change that alters behaviour should come with one.
A change that only moves code around doesn't need to.

Two rules that came out of getting them wrong: when a seam is nil, do nothing rather than
reach through it, and a plugin's teardown should remove everything its setup added.
[`docs/writing-a-plugin.md`](docs/writing-a-plugin.md) has the rest.

Bugs are equally welcome as issues: what you did, what you expected, and what happened. A
fix isn't required — knowing it's broken is the useful part.
