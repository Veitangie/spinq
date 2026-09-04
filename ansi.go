// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
	"fmt"
	"strconv"
	"strings"
)

// ANSI escape sequences for building colored FrameFuncs. spinq itself
// never emits color - these are convenience constants for callers who
// want to, via Static or a custom FrameFunc.
//
// The named colors cover the standard 8-color ANSI foreground palette
// (Black through White, SGR 30-37) plus Gray (bright black) as a common
// ninth. See RGB/Color256 for colors outside this set, and the
// Bg-prefixed constants below for background instead of foreground.
const (
	ResetStyle = "\033[0m"
	Black      = "\033[30m"
	Red        = "\033[31m"
	Green      = "\033[32m"
	Yellow     = "\033[33m"
	Blue       = "\033[34m"
	Magenta    = "\033[35m"
	Cyan       = "\033[36m"
	White      = "\033[37m"
	Gray       = "\033[90m"
)

// Background counterparts to the named foreground colors above - same
// palette, same single-parameter SGR shape, shifted to the SGR background
// range (40-47, or 100 for Gray's bright-black background).
const (
	BgBlack   = "\033[40m"
	BgRed     = "\033[41m"
	BgGreen   = "\033[42m"
	BgYellow  = "\033[43m"
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"
	BgWhite   = "\033[47m"
	BgGray    = "\033[100m"
)

// Text attributes - independent of color, freely combined with any
// constant above (or each other) via concatenation, e.g. Bold+Red.
// Terminal support varies more here than for colors - Italic and Blink in
// particular are often unsupported or reassigned.
const (
	Bold          = "\033[1m"
	Dim           = "\033[2m"
	Italic        = "\033[3m"
	Underline     = "\033[4m"
	Blink         = "\033[5m"
	Reverse       = "\033[7m"
	Hidden        = "\033[8m"
	Strikethrough = "\033[9m"
)

// Cursor visibility and line-clearing control codes. HideCursor/ShowCursor
// are convenience only - spinq itself never hides the cursor for you.
// ClearLine is the exact sequence (carriage return, then erase to end of
// line) spinq's own actor uses to redraw its line, exposed for a caller
// building a compatible custom frame.
const (
	HideCursor = "\033[?25l"
	ShowCursor = "\033[?25h"
	ClearLine  = "\r\033[K"
)

// Byte-slice forms of the ANSI constants above, for callers building frames
// as []byte without repeated string-to-[]byte conversions.
var (
	ResetStyleBytes = []byte(ResetStyle)
	BlackBytes      = []byte(Black)
	RedBytes        = []byte(Red)
	GreenBytes      = []byte(Green)
	YellowBytes     = []byte(Yellow)
	BlueBytes       = []byte(Blue)
	MagentaBytes    = []byte(Magenta)
	CyanBytes       = []byte(Cyan)
	WhiteBytes      = []byte(White)
	GrayBytes       = []byte(Gray)

	BgBlackBytes   = []byte(BgBlack)
	BgRedBytes     = []byte(BgRed)
	BgGreenBytes   = []byte(BgGreen)
	BgYellowBytes  = []byte(BgYellow)
	BgBlueBytes    = []byte(BgBlue)
	BgMagentaBytes = []byte(BgMagenta)
	BgCyanBytes    = []byte(BgCyan)
	BgWhiteBytes   = []byte(BgWhite)
	BgGrayBytes    = []byte(BgGray)

	BoldBytes          = []byte(Bold)
	DimBytes           = []byte(Dim)
	ItalicBytes        = []byte(Italic)
	UnderlineBytes     = []byte(Underline)
	BlinkBytes         = []byte(Blink)
	ReverseBytes       = []byte(Reverse)
	HiddenBytes        = []byte(Hidden)
	StrikethroughBytes = []byte(Strikethrough)

	HideCursorBytes = []byte(HideCursor)
	ShowCursorBytes = []byte(ShowCursor)
	ClearLineBytes  = []byte(ClearLine)
)

// RGB returns the 24-bit truecolor SGR escape sequence for a foreground
// color outside the named palette above, e.g. RGB(255, 128, 0) for an
// orange. Not every terminal supports truecolor; Color256 reaches more of
// them via the 256-color palette, at lower precision. See BgRGB for the
// background equivalent.
func RGB(r, g, b uint8) string {
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)
}

// BgRGB is RGB for the background instead of the foreground.
func BgRGB(r, g, b uint8) string {
	return fmt.Sprintf("\033[48;2;%d;%d;%dm", r, g, b)
}

// Color256 returns the SGR escape sequence for a foreground color, as an
// index into the ANSI 256-color palette: 0-15 are the standard/bright 16
// colors (the same 16 as the named constants above, plus their bright
// variants), 16-231 a 6x6x6 color cube, and 232-255 a 24-step grayscale
// ramp - the way to reach shades of gray beyond the single Gray constant.
// Supported on far more terminals than RGB's 24-bit truecolor. See
// BgColor256 for the background equivalent.
func Color256(color uint8) string {
	return fmt.Sprintf("\033[38;5;%dm", color)
}

// BgColor256 is Color256 for the background instead of the foreground.
func BgColor256(color uint8) string {
	return fmt.Sprintf("\033[48;5;%dm", color)
}

// Hex parses a hex color string - "#AABBCC"/"AABBCC" (leading "#"
// optional, case-insensitive) or the 3-digit CSS shorthand "#ABC" (each
// digit duplicated) - into the same escape sequence RGB would produce for
// those channel values. Any other length or non-hex character is an
// error, including an 8-digit alpha channel. See BgHex for the background
// equivalent, and HexOrEmpty for "" instead of an error.
func Hex(hex string) (string, error) {
	r, g, b, err := parseHexRGB(hex)
	if err != nil {
		return "", err
	}

	return RGB(r, g, b), nil
}

// HexOrEmpty is Hex, but returns "" instead of an error for an invalid hex
// string.
func HexOrEmpty(hex string) string {
	res, err := Hex(hex)
	if err != nil {
		res = ""
	}
	return res
}

// BgHex is Hex for the background instead of the foreground.
func BgHex(hex string) (string, error) {
	r, g, b, err := parseHexRGB(hex)
	if err != nil {
		return "", err
	}

	return BgRGB(r, g, b), nil
}

// BgHexOrEmpty is HexOrEmpty for the background instead of the foreground.
func BgHexOrEmpty(hex string) string {
	res, err := BgHex(hex)
	if err != nil {
		res = ""
	}
	return res
}

func parseHexRGB(hex string) (uint8, uint8, uint8, error) {
	hex, _ = strings.CutPrefix(hex, "#")
	if len(hex) != 3 && len(hex) != 6 {
		return 0, 0, 0, fmt.Errorf("invalid rgb hex length: %d, expected 3 or 6", len(hex))
	}

	var rString, gString, bString string
	if len(hex) == 3 {
		rString = hex[0:1]
		gString = hex[1:2]
		bString = hex[2:3]
	} else {
		rString = hex[0:2]
		gString = hex[2:4]
		bString = hex[4:6]
	}

	r, err := parseHex(rString)
	if err != nil {
		return 0, 0, 0, err
	}
	g, err := parseHex(gString)
	if err != nil {
		return 0, 0, 0, err
	}
	b, err := parseHex(bString)
	if err != nil {
		return 0, 0, 0, err
	}

	return r, g, b, nil
}

func parseHex(hex string) (uint8, error) {
	res64, err := strconv.ParseUint(hex, 16, 8)
	if len(hex) == 1 {
		res64 = (res64 << 4) + res64
	}
	return uint8(res64), err
}
