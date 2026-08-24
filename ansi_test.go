// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import (
	"fmt"
	"regexp"
	"testing"
)

var sgrColorConstant = regexp.MustCompile(`^\033\[[0-9]+(;[0-9]+)*m$`)

func TestNamedColorConstants_AreWellFormedEscapeSequences(t *testing.T) {
	colors := map[string]string{
		"Black":   Black,
		"Red":     Red,
		"Green":   Green,
		"Yellow":  Yellow,
		"Blue":    Blue,
		"Magenta": Magenta,
		"Cyan":    Cyan,
		"White":   White,
		"Gray":    Gray,

		"BgBlack":   BgBlack,
		"BgRed":     BgRed,
		"BgGreen":   BgGreen,
		"BgYellow":  BgYellow,
		"BgBlue":    BgBlue,
		"BgMagenta": BgMagenta,
		"BgCyan":    BgCyan,
		"BgWhite":   BgWhite,
		"BgGray":    BgGray,
	}

	for name, seq := range colors {
		t.Run(name, func(t *testing.T) {
			if !sgrColorConstant.MatchString(seq) {
				t.Errorf("%s = %q does not match the expected SGR escape sequence shape (\\033[<n>m)", name, seq)
			}
		})
	}
}

func TestBackgroundColorConstants_MatchForegroundCodePlusTen(t *testing.T) {
	var sgrCode = regexp.MustCompile(`^\033\[([0-9]+)m$`)

	codeOf := func(t *testing.T, seq string) int {
		t.Helper()
		m := sgrCode.FindStringSubmatch(seq)
		if m == nil {
			t.Fatalf("%q is not a single-parameter SGR sequence", seq)
		}
		var n int
		if _, err := fmt.Sscanf(m[1], "%d", &n); err != nil {
			t.Fatalf("parsing SGR code from %q: %v", seq, err)
		}
		return n
	}

	pairs := []struct {
		name   string
		fg, bg string
	}{
		{"Black", Black, BgBlack},
		{"Red", Red, BgRed},
		{"Green", Green, BgGreen},
		{"Yellow", Yellow, BgYellow},
		{"Blue", Blue, BgBlue},
		{"Magenta", Magenta, BgMagenta},
		{"Cyan", Cyan, BgCyan},
		{"White", White, BgWhite},
		{"Gray", Gray, BgGray},
	}

	for _, p := range pairs {
		t.Run(p.name, func(t *testing.T) {
			fgCode, bgCode := codeOf(t, p.fg), codeOf(t, p.bg)
			if bgCode != fgCode+10 {
				t.Errorf("Bg%s = %q (code %d), want code %d (%s's code %d + 10)", p.name, p.bg, bgCode, fgCode+10, p.name, fgCode)
			}
		})
	}
}

func TestRGB(t *testing.T) {
	for _, tc := range []struct {
		r, g, b uint8
		want    string
	}{
		{255, 0, 0, "\033[38;2;255;0;0m"},
		{0, 0, 0, "\033[38;2;0;0;0m"},
	} {
		t.Run(fmt.Sprintf("%d,%d,%d", tc.r, tc.g, tc.b), func(t *testing.T) {
			if got := RGB(tc.r, tc.g, tc.b); got != tc.want {
				t.Errorf("RGB(%d, %d, %d) = %q, want %q", tc.r, tc.g, tc.b, got, tc.want)
			}
		})
	}
}

func TestBgRGB(t *testing.T) {
	for _, tc := range []struct {
		r, g, b uint8
		want    string
	}{
		{255, 0, 0, "\033[48;2;255;0;0m"},
		{0, 0, 0, "\033[48;2;0;0;0m"},
	} {
		t.Run(fmt.Sprintf("%d,%d,%d", tc.r, tc.g, tc.b), func(t *testing.T) {
			if got := BgRGB(tc.r, tc.g, tc.b); got != tc.want {
				t.Errorf("BgRGB(%d, %d, %d) = %q, want %q", tc.r, tc.g, tc.b, got, tc.want)
			}
		})
	}
}

func TestColor256(t *testing.T) {
	for _, tc := range []struct {
		color uint8
		want  string
	}{
		{0, "\033[38;5;0m"},
		{255, "\033[38;5;255m"},
	} {
		t.Run(fmt.Sprintf("%d", tc.color), func(t *testing.T) {
			if got := Color256(tc.color); got != tc.want {
				t.Errorf("Color256(%d) = %q, want %q", tc.color, got, tc.want)
			}
		})
	}
}

func TestBgColor256(t *testing.T) {
	for _, tc := range []struct {
		color uint8
		want  string
	}{
		{0, "\033[48;5;0m"},
		{255, "\033[48;5;255m"},
	} {
		t.Run(fmt.Sprintf("%d", tc.color), func(t *testing.T) {
			if got := BgColor256(tc.color); got != tc.want {
				t.Errorf("BgColor256(%d) = %q, want %q", tc.color, got, tc.want)
			}
		})
	}
}

func TestHex_SixDigit(t *testing.T) {
	for _, tc := range []struct {
		hex     string
		r, g, b uint8
	}{
		{"#AABBCC", 0xAA, 0xBB, 0xCC},
		{"AABBCC", 0xAA, 0xBB, 0xCC},
		{"#aabbcc", 0xAA, 0xBB, 0xCC},
		{"#AaBbCc", 0xAA, 0xBB, 0xCC},
		{"#000000", 0, 0, 0},
		{"#ffffff", 255, 255, 255},
	} {
		t.Run(tc.hex, func(t *testing.T) {
			want := RGB(tc.r, tc.g, tc.b)
			got, err := Hex(tc.hex)
			if err != nil {
				t.Fatalf("Hex(%q): unexpected error: %v", tc.hex, err)
			}
			if got != want {
				t.Errorf("Hex(%q) = %q, want %q", tc.hex, got, want)
			}
		})
	}
}

func TestHex_ThreeDigitShorthandDoublesEachDigit(t *testing.T) {
	for _, tc := range []struct {
		hex     string
		r, g, b uint8
	}{
		{"#fff", 0xff, 0xff, 0xff},
		{"#000", 0x00, 0x00, 0x00},
		{"#f00", 0xff, 0x00, 0x00},
		{"#0f0", 0x00, 0xff, 0x00},
		{"#00f", 0x00, 0x00, 0xff},
		{"#a1b", 0xaa, 0x11, 0xbb},
	} {
		t.Run(tc.hex, func(t *testing.T) {
			want := RGB(tc.r, tc.g, tc.b)
			got, err := Hex(tc.hex)
			if err != nil {
				t.Fatalf("Hex(%q): unexpected error: %v", tc.hex, err)
			}
			if got != want {
				t.Errorf("Hex(%q) = %q, want %q (each digit should be duplicated, e.g. 'f' -> 'ff')", tc.hex, got, want)
			}
		})
	}
}

func TestHex_InvalidInput(t *testing.T) {
	for _, hex := range []string{
		"",
		"#",
		"1",
		"12",
		"1234",
		"12345",
		"1234567",
		"12345678",
		"GGGGGG",
		"ZZZ",
		"#12345g",
	} {
		t.Run(hex, func(t *testing.T) {
			if _, err := Hex(hex); err == nil {
				t.Errorf("Hex(%q): expected an error, got nil", hex)
			}
		})
	}
}

func TestHexOrEmpty(t *testing.T) {
	if got := HexOrEmpty("#AABBCC"); got != RGB(0xAA, 0xBB, 0xCC) {
		t.Errorf("HexOrEmpty(valid) = %q, want %q", got, RGB(0xAA, 0xBB, 0xCC))
	}
	if got := HexOrEmpty("not-a-color"); got != "" {
		t.Errorf("HexOrEmpty(invalid) = %q, want empty string", got)
	}
}

func TestBgHex_SixDigit(t *testing.T) {
	want := BgRGB(0xAA, 0xBB, 0xCC)
	got, err := BgHex("#AABBCC")
	if err != nil {
		t.Fatalf("BgHex: unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("BgHex(%q) = %q, want %q", "#AABBCC", got, want)
	}
}

func TestBgHex_InvalidInput(t *testing.T) {
	if _, err := BgHex("not-a-color"); err == nil {
		t.Error("BgHex(invalid): expected an error, got nil")
	}
}

func TestBgHexOrEmpty(t *testing.T) {
	if got := BgHexOrEmpty("#AABBCC"); got != BgRGB(0xAA, 0xBB, 0xCC) {
		t.Errorf("BgHexOrEmpty(valid) = %q, want %q", got, BgRGB(0xAA, 0xBB, 0xCC))
	}
	if got := BgHexOrEmpty("not-a-color"); got != "" {
		t.Errorf("BgHexOrEmpty(invalid) = %q, want empty string", got)
	}
}
