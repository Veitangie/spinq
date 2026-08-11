# Renders this directory's demo into progress-bar.gif.
#
# Run from inside examples/progress-bar/:
#   vhs progress-bar.tape
#
# Requires the vhs CLI: https://github.com/charmbracelet/vhs

Output progress-bar.gif

Require go

Set Shell "zsh"
Set FontSize 24
Set FontFamily "FiraCode Nerd Font Mono"
Set Width 900
Set Height 500
Set Padding 20
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set BorderRadius 20

Hide
Type "clear; printf '\n%.0s' {1..40}"
Enter
Sleep 500ms
Show

Type "go run ."
Enter

Sleep 6s
