// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import (
	"slices"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestNoopRender(t *testing.T) {
	got := NoopRender()(5, 10)
	if len(got) != 0 {
		t.Errorf("expected no bytes, got %q", got)
	}
}

func TestRenderFunc_Join(t *testing.T) {
	a := RenderFunc(func(current, total int) []byte { return []byte("a") })
	b := RenderFunc(func(current, total int) []byte { return []byte("b") })

	got := a.Join("-", b)(1, 2)
	if string(got) != "a-b" {
		t.Errorf("expected %q, got %q", "a-b", got)
	}
}

func TestRenderFunc_Join_NilOtherReturnsUnchanged(t *testing.T) {
	a := RenderFunc(func(current, total int) []byte { return []byte("a") })

	got := a.Join("-", nil)(1, 2)
	if string(got) != "a" {
		t.Errorf("expected the receiver unchanged when other is nil, got %q", got)
	}
}

func TestJoinRender_JoinsMultiple(t *testing.T) {
	a := RenderFunc(func(current, total int) []byte { return []byte("a") })
	b := RenderFunc(func(current, total int) []byte { return []byte("b") })
	c := RenderFunc(func(current, total int) []byte { return []byte("c") })

	got := JoinRender(" ", a, b, c)(1, 2)
	if string(got) != "a b c" {
		t.Errorf("expected %q, got %q", "a b c", got)
	}
}

func TestJoinRender_Empty(t *testing.T) {
	got := JoinRender(" ")(1, 2)
	if len(got) != 0 {
		t.Errorf("expected no bytes for an empty Join, got %q", got)
	}
}

func TestJoinRender_AllNil(t *testing.T) {
	got := JoinRender(" ", nil, nil)(1, 2)
	if len(got) != 0 {
		t.Errorf("expected no bytes when every func is nil, got %q", got)
	}
}

func TestJoinRender_KeepsNonNilFuncsWhenSomeAreNil(t *testing.T) {
	a := RenderFunc(func(current, total int) []byte { return []byte("a") })
	b := RenderFunc(func(current, total int) []byte { return []byte("b") })

	got := JoinRender(" ", a, nil, b)(1, 2)
	if string(got) != "a b" {
		t.Errorf("expected %q, got %q", "a b", got)
	}
}

func TestBarRender_RendersAtHalfway(t *testing.T) {
	got := BarRender(10)(5, 10)
	if string(got) != "[===>    ]" {
		t.Errorf("expected %q, got %q", "[===>    ]", got)
	}
}

func TestBarRender_RendersAtStart(t *testing.T) {
	got := BarRender(10)(0, 10)
	if string(got) != "[>       ]" {
		t.Errorf("expected %q, got %q", "[>       ]", got)
	}
}

func TestBarRender_FullWidthMatchesOtherLevels(t *testing.T) {
	r := BarRender(10)
	full := r(10, 10)
	half := r(5, 10)

	if len(full) != len(half) {
		t.Errorf("expected the bar to render at a constant width regardless of progress, got len=%d at 100%% vs len=%d at 50%% (%q vs %q)",
			len(full), len(half), full, half)
	}
}

func TestBarRender_CustomOptions(t *testing.T) {
	opts := BarOptions{Start: "<", Full: "#", Divider: "|", Empty: ".", End: ">"}
	got := BarRender(10, WithBarOptions(opts))(5, 10)
	if string(got) != "<###|....>" {
		t.Errorf("expected %q, got %q", "<###|....>", got)
	}
}

func TestBarRender_LengthTooSmallReturnsNoop(t *testing.T) {
	got := BarRender(3)(5, 10)
	if len(got) != 0 {
		t.Errorf("expected no bytes when length leaves no room for the bar, got %q", got)
	}
}

func TestBarRender_MismatchedUnitWidthsReturnsNoop(t *testing.T) {
	opts := BarOptions{Start: "[", Full: "==", Divider: ">", Empty: " ", End: "]"}
	got := BarRender(10, WithBarOptions(opts))(5, 10)
	if len(got) != 0 {
		t.Errorf("expected no bytes when Full and Empty have different widths, got %q", got)
	}
}

func TestBarRender_DirectionLeftMirrorsRight(t *testing.T) {
	opts := func(dir RenderDirection) BarOptions {
		return BarOptions{Start: "[", Full: "=", Divider: ">", Empty: " ", End: "]", Direction: dir}
	}

	for _, tc := range []struct {
		current, total      int
		wantRight, wantLeft string
	}{
		{0, 10, "[>       ]", "[       >]"},
		{5, 10, "[===>    ]", "[    >===]"},
		{9, 10, "[======> ]", "[ >======]"},
		{10, 10, "[=======>]", "[>=======]"},
	} {
		gotRight := BarRender(10, WithBarOptions(opts(Right)))(tc.current, tc.total)
		if string(gotRight) != tc.wantRight {
			t.Errorf("current=%d total=%d Right: expected %q, got %q", tc.current, tc.total, tc.wantRight, gotRight)
		}

		gotLeft := BarRender(10, WithBarOptions(opts(Left)))(tc.current, tc.total)
		if string(gotLeft) != tc.wantLeft {
			t.Errorf("current=%d total=%d Left: expected %q, got %q", tc.current, tc.total, tc.wantLeft, gotLeft)
		}
	}
}

func TestFractRender(t *testing.T) {
	got := FractRender("/")(5, 10)
	if string(got) != "5/10" {
		t.Errorf("expected %q, got %q", "5/10", got)
	}
}

func TestFractRender_CustomSeparator(t *testing.T) {
	got := FractRender(" of ")(5, 10)
	if string(got) != "5 of 10" {
		t.Errorf("expected %q, got %q", "5 of 10", got)
	}
}

func TestPercentRender(t *testing.T) {
	for _, tc := range []struct {
		current, total int
		want           string
	}{
		{0, 10, "0%"},
		{5, 10, "50%"},
		{10, 10, "100%"},
		{1, 3, "33%"},
	} {
		got := PercentRender()(tc.current, tc.total)
		if string(got) != tc.want {
			t.Errorf("current=%d total=%d: expected %q, got %q", tc.current, tc.total, tc.want, got)
		}
	}
}

func TestBarOptionsPresets(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts BarOptions
		want string
	}{
		{"Rounded", RoundedBarOptions(), "(###>------)"},
		{"Shade", ShadeBarOptions(), "█████░░░░░░░"},
		{"Dot", DotBarOptions(), "(●●●●○○○○○○)"},
		{"Minimal", MinimalBarOptions(), "####>-------"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := BarRender(12, WithBarOptions(tc.opts))(4, 10)
			if string(got) != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestSmoothBarRender_RendersAtVariousLevels(t *testing.T) {
	r := SmoothBarRender(18)
	for _, tc := range []struct {
		current int
		want    string
	}{
		{0, "▏                 "},
		{2, "███▋              "},
		{5, "█████████▏        "},
		{8, "██████████████▌   "},
		{10, "██████████████████"},
	} {
		got := r(tc.current, 10)
		if string(got) != tc.want {
			t.Errorf("current=%d: expected %q, got %q", tc.current, tc.want, got)
		}
	}
}

func TestSmoothBarRender_WidthConstantAcrossLevels(t *testing.T) {
	r := SmoothBarRender(18)
	want := runewidth.StringWidth(string(r(5, 10)))
	for _, current := range []int{0, 1, 3, 7, 9, 10} {
		if got := runewidth.StringWidth(string(r(current, 10))); got != want {
			t.Errorf("current=%d: expected width %d (matching current=5), got %d for %q", current, want, got, r(current, 10))
		}
	}
}

func TestSmoothBarRender_CustomDividers(t *testing.T) {
	got := SmoothBarRender(10, SmoothWithDivider([]string{"a", "b", "c", "d"}))(5, 10)
	if len(got) == 0 {
		t.Fatal("expected non-empty output")
	}
	if !strings.ContainsAny(string(got), "abcd") {
		t.Errorf("expected one of the custom dividers to appear, got %q", got)
	}
}

func TestSmoothBarRender_MismatchedDividerWidthsReturnsNoop(t *testing.T) {
	got := SmoothBarRender(18, SmoothWithDivider([]string{"a", "ab", "c", "d"}))(5, 10)
	if len(got) != 0 {
		t.Errorf("expected no bytes when dividers have inconsistent widths among themselves, got %q", got)
	}
}

func TestSmoothBarRender_DividerWidthMustMatchFullAndEmpty(t *testing.T) {
	got := SmoothBarRender(18, SmoothWithDivider([]string{"ab", "cd", "ef"}))(5, 10)
	if len(got) != 0 {
		t.Errorf("expected no bytes when dividers are wider than Full/Empty, got %q", got)
	}
}

func TestSmoothBarRender_LengthTooSmallReturnsNoop(t *testing.T) {
	got := SmoothBarRender(0)(5, 10)
	if len(got) != 0 {
		t.Errorf("expected no bytes when length leaves no room for the bar, got %q", got)
	}
}

func TestSmoothBarRender_OneDividerFallsBackToPlainBarWithThatDivider(t *testing.T) {
	got := SmoothBarRender(10, SmoothWithDivider([]string{"X"}))(5, 10)
	if !strings.Contains(string(got), "X") {
		t.Errorf("expected the single divider to appear, got %q", got)
	}
}

func TestSmoothBarRender_ZeroDividersFallsBackToPlainBarWithNoDivider(t *testing.T) {
	got := SmoothBarRender(10, SmoothWithDivider(nil))(5, 10)
	if string(got) != "█████     " {
		t.Errorf("expected a plain bar with no divider character, got %q", got)
	}
}

func TestSmoothBarRender_TwoDividersAreNotSilentlyDropped(t *testing.T) {
	got := SmoothBarRender(10, SmoothWithDivider([]string{"A", "B"}))(5, 10)
	noDividers := SmoothBarRender(10, SmoothWithDivider(nil))(5, 10)

	if string(got) == string(noDividers) {
		t.Errorf("expected two explicitly provided dividers to have some effect on the output, but got %q — identical to passing zero dividers %q", got, noDividers)
	}
}

func TestSmoothBarRender_DirectionLeftShowsDividerAtZero(t *testing.T) {
	right := SmoothBarRender(18)(0, 10)
	left := SmoothBarRender(18, SmoothWithDirection(Left))(0, 10)

	if string(right) == "[                ]" {
		t.Fatal("test premise broken: Right no longer shows a divider at current == 0")
	}
	if string(left) == "[                ]" {
		t.Errorf("expected Direction: Left to show a boundary divider at current == 0, matching Right's behavior at its own boundary, got a blank bar %q", left)
	}
}

func reverseRunes(s string) string {
	r := []rune(s)
	slices.Reverse(r)
	return string(r)
}

func TestSmoothBarRender_DirectionLeftMirrorsRight(t *testing.T) {
	right := SmoothBarRender(18)
	left := SmoothBarRender(18, SmoothWithDirection(Left))

	for current := 0; current <= 10; current++ {
		gotRight := string(right(current, 10))
		gotLeft := string(left(current, 10))

		wantLeft := reverseRunes(gotRight)
		if gotLeft != wantLeft {
			t.Errorf("current=%d: Left %q is not the character-reversal of Right %q", current, gotLeft, gotRight)
		}
	}
}

func TestBarOptionsFuncs_SetIndividualFields(t *testing.T) {
	opt := DefaultBarOptions()
	opt = BarWithStart("<")(opt)
	opt = BarWithFull("#")(opt)
	opt = BarWithDivider("|")(opt)
	opt = BarWithEmpty(".")(opt)
	opt = BarWithEnd(">")(opt)
	opt = BarWithDirection(Left)(opt)

	want := BarOptions{Start: "<", Full: "#", Divider: "|", Empty: ".", End: ">", Direction: Left}
	if opt != want {
		t.Errorf("got %+v, want %+v", opt, want)
	}
}

func TestSmoothBarOptionsFuncs_SetIndividualFields(t *testing.T) {
	opt := DefaultSmoothBarOptions()
	opt = SmoothWithStart("<")(opt)
	opt = SmoothWithFull("#")(opt)
	opt = SmoothWithEmpty(".")(opt)
	opt = SmoothWithEnd(">")(opt)

	if opt.Start != "<" {
		t.Errorf("SmoothWithStart: got Start=%q, want %q", opt.Start, "<")
	}
	if opt.Full != "#" {
		t.Errorf("SmoothWithFull: got Full=%q, want %q", opt.Full, "#")
	}
	if opt.Empty != "." {
		t.Errorf("SmoothWithEmpty: got Empty=%q, want %q", opt.Empty, ".")
	}
	if opt.End != ">" {
		t.Errorf("SmoothWithEnd: got End=%q, want %q", opt.End, ">")
	}
}

func TestSmoothWithOptions_ReplacesWholeStruct(t *testing.T) {
	opt := SmoothWithStart("previous")(DefaultSmoothBarOptions())
	replacement := SmoothBarOptions{
		Start:     "<",
		Full:      "#",
		Dividers:  []string{"o"},
		Empty:     ".",
		End:       ">",
		Direction: Right,
	}

	opt = WithSmoothOptions(replacement)(opt)

	if opt.Start != replacement.Start {
		t.Errorf("expected SmoothWithOptions to fully replace the struct; got Start=%q, want %q (the earlier SmoothWithStart value should be gone)", opt.Start, replacement.Start)
	}
	if opt.Full != replacement.Full || opt.Empty != replacement.Empty || opt.End != replacement.End {
		t.Errorf("expected the replacement struct's fields verbatim, got %+v", opt)
	}
	if len(opt.Dividers) != 1 || opt.Dividers[0] != "o" {
		t.Errorf("expected Dividers to come from the replacement struct, got %v", opt.Dividers)
	}
}
