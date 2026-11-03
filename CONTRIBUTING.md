# Contributing

## Opening a pull request

Branch, change, pull request. Nothing goes straight to `main`.

Keep a pull request to one thing. A fix and a refactor in the same branch is two
conversations happening at once.

## Commits

Say what changed and why it had to. `Fix the config path` is enough. A commit message is a
record for whoever reads it next, so write the reason, not the mood.

## Before you send it

```sh
make check
```

That's gofmt, vet, every test, and a Windows build — the same thing CI runs. If it passes
here it should pass there.

## Tests

Tests live beside the code they test. A change that alters behaviour should come with one;
a change that only moves code around doesn't need to.

## Writing plugins

[`docs/writing-a-plugin.md`](docs/writing-a-plugin.md) covers the plugin contract, the seams
available, and the rules that came out of getting them wrong. `plugins/greet/` is the
smallest complete plugin.

## Code

- Comments explain what the code is for, and why it's shaped that way. Not what the next
  line does.
- If a seam is nil, do nothing rather than reach through it. A plugin that isn't given a
  capability shouldn't assume it has one.
- Take back what you offered. A plugin's teardown should remove everything its setup added.

## Reporting a bug

Open an issue with what you did, what you expected, and what happened. A fix is welcome but
not required — knowing it's broken is the useful part.
