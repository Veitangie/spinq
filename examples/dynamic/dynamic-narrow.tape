# Renders this directory's demo into dynamic-narrow.gif, at a narrow
# terminal width. Run dynamic-wide.tape too, to see the same command adapt
# to a much wider one - vhs can't capture a live resize inside one
# recording, so two fixed-width tapes are the closest equivalent: same
# command, two terminal widths, two different bars.
#
# Run from inside examples/dynamic/:
#   vhs dynamic-narrow.tape
#
# Requires the vhs CLI: https://github.com/charmbracelet/vhs

Output dynamic-narrow.gif

Require go

Set Shell "zsh"
Set FontSize 20
Set FontFamily "FiraCode Nerd Font Mono"
Set Width 700
Set Height 300
Set Padding 20
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set BorderRadius 10

Hide
Type "clear; printf '\n%.0s' {1..40}"
Enter
Sleep 500ms
Show

Type "go run ."
Enter

Sleep 4s
