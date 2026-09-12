# Contributing to spinq

## Scope

spinq stays intentionally narrow - see "What spinq does not do" in the
README for the reasoning behind each of these. The following are out of
scope and won't be accepted as feature requests or PRs:

- Raw/true TTY mode, reading input, or anything TUI-framework-shaped.
- Automatic terminal-width detection. Bar widths stay explicit `int`s -
  build a width source yourself (`WidthFromFile`, `CachedGetWidth`) or use
  `DefaultGetWidth`.
- Multiline or multi-bar dashboards. spinq manages a single line, on
  purpose.
- Terminfo/termcap parsing or non-ANSI fallback rendering. Detection stays
  `isatty`-based.

If your use case needs one of these, spinq's primitives are meant to be
composable enough to build it yourself outside the package - see
"Composing frames" and "What spinq does not do" in the README.

## Versioning, from a PR's perspective

spinq follows semver judged strictly from the calling code's perspective -
see "Versioning" in the README for the exact patch/minor/major rules. In
short: a behavior change on an exported function is a major bump even
when nothing fails to compile, if it breaks a guarantee the doc comment
made. If your change touches an exported identifier's documented
behavior, call that out explicitly in the PR description rather than
leaving it to be inferred from the diff.

## Before submitting a PR

Open an issue first for anything beyond a small, obvious fix - what
you're proposing and why. That saves you from writing a PR against
something that turns out to be out of scope, and saves both of us from a
PR sitting unreviewed because it needed a discussion first.
