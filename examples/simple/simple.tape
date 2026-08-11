# Renders this directory's demo into simple.gif.
#
# Run from inside examples/simple/:
#   vhs simple.tape
#
# Requires the vhs CLI: https://github.com/charmbracelet/vhs

Output simple.gif

Require go

Set Shell "zsh"
Set FontSize 36
Set FontFamily "FiraCode Nerd Font Mono"
Set Width 900
Set Height 500
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
