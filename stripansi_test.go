// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import "testing"

func TestStripANSI_NoEscapeSequencesIsUnchanged(t *testing.T) {
	for _, s := range []string{"", "plain text", "line one\nline two"} {
		if got := StripANSI(s); got != s {
			t.Errorf("StripANSI(%q) = %q, want unchanged", s, got)
		}
	}
}

func TestStripANSI_RemovesNamedColorConstants(t *testing.T) {
	styled := Red + "error" + ResetStyle
	if got, want := StripANSI(styled), "error"; got != want {
		t.Errorf("StripANSI(%q) = %q, want %q", styled, got, want)
	}
}

func TestStripANSI_RemovesConsecutiveSequences(t *testing.T) {
	styled := Bold + Red + "urgent" + ResetStyle
	if got, want := StripANSI(styled), "urgent"; got != want {
		t.Errorf("StripANSI(%q) = %q, want %q", styled, got, want)
	}
}

func TestStripANSI_RemovesRGBAndHexSequences(t *testing.T) {
	styled := RGB(255, 0, 0) + "reddish" + ResetStyle
	if got, want := StripANSI(styled), "reddish"; got != want {
		t.Errorf("StripANSI(%q) = %q, want %q", styled, got, want)
	}

	hexSeq, err := Hex("#ff8800")
	if err != nil {
		t.Fatalf("Hex: %v", err)
	}
	styled = hexSeq + "amber" + ResetStyle
	if got, want := StripANSI(styled), "amber"; got != want {
		t.Errorf("StripANSI(%q) = %q, want %q", styled, got, want)
	}
}

func TestStripANSI_RemovesCursorSequences(t *testing.T) {
	styled := HideCursor + "spinning" + ShowCursor
	if got, want := StripANSI(styled), "spinning"; got != want {
		t.Errorf("StripANSI(%q) = %q, want %q", styled, got, want)
	}
}

func TestStripANSIBytes_MatchesStripANSI(t *testing.T) {
	for _, s := range []string{
		"",
		"plain",
		Red + "colored" + ResetStyle,
		Bold + Underline + "styled" + ResetStyle,
	} {
		want := StripANSI(s)
		if got := string(StripANSIBytes([]byte(s))); got != want {
			t.Errorf("StripANSIBytes(%q) = %q, want %q (StripANSI's result)", s, got, want)
		}
	}
}
