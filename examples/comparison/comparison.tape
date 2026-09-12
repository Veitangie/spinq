# Renders the README's top-of-page comparison GIF: the same fake pipeline
# run twice, once through a hand-rolled naive spinner (no coordination with
# the stdout writes happening alongside it) and once through spinq. Output
# goes straight to the assets repo, not into this repo, so it isn't
# duplicated here as well.
#
# Run from inside examples/comparison/:
#   vhs comparison.tape
#
# The Output line below is only a default - it assumes the assets repo is
# checked out as a sibling of this one (matching .github/workflows/gremlins.yml
# in CI). If yours lives elsewhere, override it with vhs's own -o/--output
# flag, which takes precedence over the Output line entirely (export ASSETS
# on its own line first - a same-line "ASSETS=... vhs ... -o \"$ASSETS/...\""
# won't see the assignment, since the shell expands "$ASSETS" in that same
# command's own arguments before the prefix assignment takes effect):
#   export ASSETS=/path/to/assets
#   vhs comparison.tape -o "$ASSETS/demos/spinq/comparison.gif"
#
# Requires the vhs CLI: https://github.com/charmbracelet/vhs

Output ../../../assets/demos/spinq/comparison.gif

Require go

Set Shell "zsh"
Set FontSize 18
Set FontFamily "FiraCode Nerd Font Mono"
Set Width 1000
Set Height 965
Set Padding 20
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set BorderRadius 20

Hide
Type "go build -o comparison ."
Enter
Sleep 2s
Type "clear; printf '\n%.0s' {1..100}"
Enter
Sleep 500ms
Show

Type "echo '--- no coordination ---' && ./comparison naive"
Enter
Sleep 4s

Type "echo '--- the spinq approach ---' && ./comparison spinq"
Enter
Sleep 7s
