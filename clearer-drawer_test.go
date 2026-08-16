// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import (
	"bytes"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

func newRunning(v bool) *atomic.Bool {
	b := &atomic.Bool{}
	b.Store(v)
	return b
}

func TestAwareClearerDrawer_ClearMess_ZeroWidthFromGetWidth_DoesNotPanic(t *testing.T) {
	a := &awareClearerDrawer{
		width:    40,
		getWidth: func() int { return 0 },
		visible:  []byte("some previously drawn frame"),
	}

	st := &spinnerState{
		wrapped:   &bytes.Buffer{},
		needClear: true,
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("clear panicked with getWidth() returning 0: %v", r)
		}
	}()

	if err := a.clear(st); err != nil {
		t.Fatalf("clear returned error: %v", err)
	}
}

func TestObliviousClearerDrawer_Adjust_IsNoop(t *testing.T) {
	obliviousClearerDrawer{}.adjust(&spinnerState{})
}

func TestAwareClearerDrawer_Draw(t *testing.T) {
	t.Run("not running", func(t *testing.T) {
		buf := &bytes.Buffer{}
		a := &awareClearerDrawer{width: 40}
		st := &spinnerState{wrapped: buf, running: newRunning(false), canWrite: true, frame: []byte("hi")}
		if err := a.draw(st); err != nil {
			t.Fatalf("draw: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("expected no write when not running, got %q", buf.String())
		}
	})

	t.Run("cannot write", func(t *testing.T) {
		buf := &bytes.Buffer{}
		a := &awareClearerDrawer{width: 40}
		st := &spinnerState{wrapped: buf, running: newRunning(true), canWrite: false, frame: []byte("hi")}
		if err := a.draw(st); err != nil {
			t.Fatalf("draw: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("expected no write when canWrite is false, got %q", buf.String())
		}
	})

	t.Run("needs clear first", func(t *testing.T) {
		buf := &bytes.Buffer{}
		a := &awareClearerDrawer{width: 40}
		st := &spinnerState{wrapped: buf, running: newRunning(true), canWrite: true, needClear: true, frame: []byte("hi")}
		if err := a.draw(st); err != nil {
			t.Fatalf("draw: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("expected no write while needClear is true, got %q", buf.String())
		}
	})

	t.Run("empty frame", func(t *testing.T) {
		buf := &bytes.Buffer{}
		a := &awareClearerDrawer{width: 40}
		st := &spinnerState{wrapped: buf, running: newRunning(true), canWrite: true}
		if err := a.draw(st); err != nil {
			t.Fatalf("draw: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("expected no write for an empty frame, got %q", buf.String())
		}
	})

	t.Run("width unknown", func(t *testing.T) {
		buf := &bytes.Buffer{}
		a := &awareClearerDrawer{width: 0}
		st := &spinnerState{wrapped: buf, running: newRunning(true), canWrite: true, frame: []byte("hi")}
		if err := a.draw(st); err != nil {
			t.Fatalf("draw: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("expected no write while width is unknown, got %q", buf.String())
		}
		if st.needClear {
			t.Error("expected needClear to stay false while width is unknown - the guard should block draw entirely, not just produce an empty write")
		}
	})

	t.Run("draws precomputed visible and marks needClear", func(t *testing.T) {
		buf := &bytes.Buffer{}
		a := &awareClearerDrawer{width: 40, visible: []byte("precomputed")}
		st := &spinnerState{wrapped: buf, running: newRunning(true), canWrite: true, frame: []byte("hi")}
		if err := a.draw(st); err != nil {
			t.Fatalf("draw: %v", err)
		}
		if buf.String() != "precomputed" {
			t.Errorf("expected the precomputed visible bytes to be written verbatim, got %q", buf.String())
		}
		if !st.needClear {
			t.Error("expected draw to mark needClear")
		}
	})

	t.Run("lazily computes visible from frame when not yet set", func(t *testing.T) {
		buf := &bytes.Buffer{}
		a := &awareClearerDrawer{width: 40}
		st := &spinnerState{wrapped: buf, running: newRunning(true), canWrite: true, frame: []byte("hi")}
		if err := a.draw(st); err != nil {
			t.Fatalf("draw: %v", err)
		}
		if buf.String() != "hi" {
			t.Errorf("expected draw to lazily adjust() from st.frame and write the result, got %q", buf.String())
		}
	})

	t.Run("propagates write error", func(t *testing.T) {
		a := &awareClearerDrawer{width: 40, visible: []byte("x")}
		st := &spinnerState{wrapped: errWriter{err: errors.New("boom")}, running: newRunning(true), canWrite: true, frame: []byte("hi")}
		if err := a.draw(st); err == nil {
			t.Error("expected draw to propagate the underlying write error")
		}
	})
}

func TestAwareClearerDrawer_ClearMess(t *testing.T) {
	t.Run("nothing to clear", func(t *testing.T) {
		buf := &bytes.Buffer{}
		a := &awareClearerDrawer{width: 40}
		st := &spinnerState{wrapped: buf, needClear: false}
		if err := a.clearMess(st); err != nil {
			t.Fatalf("clearMess: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("expected no write when nothing needs clearing, got %q", buf.String())
		}
	})

	t.Run("zero width is a no-op, not a panic", func(t *testing.T) {
		buf := &bytes.Buffer{}
		a := &awareClearerDrawer{width: 0, visible: []byte("stale")}
		st := &spinnerState{wrapped: buf, needClear: true}
		if err := a.clearMess(st); err != nil {
			t.Fatalf("clearMess: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("expected no write when width is unknown (0), got %q", buf.String())
		}
		if !st.needClear {
			t.Error("expected needClear to stay true when width is unknown, so a later real clear still happens")
		}
	})

	t.Run("single line fits within current width", func(t *testing.T) {
		buf := &bytes.Buffer{}
		a := &awareClearerDrawer{width: 40, visible: []byte("short")}
		st := &spinnerState{wrapped: buf, needClear: true}
		if err := a.clearMess(st); err != nil {
			t.Fatalf("clearMess: %v", err)
		}
		if got := buf.String(); got != string(clearBytes) {
			t.Errorf("expected exactly one clearBytes sequence, got %q", got)
		}
		if st.needClear {
			t.Error("expected needClear to be cleared after writing")
		}
	})

	t.Run("stale content wider than current width clears multiple lines, rounding up", func(t *testing.T) {
		buf := &bytes.Buffer{}
		wide := bytes.Repeat([]byte("x"), 35)
		a := &awareClearerDrawer{width: 10, visible: wide}
		st := &spinnerState{wrapped: buf, needClear: true}
		if err := a.clearMess(st); err != nil {
			t.Fatalf("clearMess: %v", err)
		}
		want := string(clearBytes) + strings.Repeat(string(clearPrevLine), 3)
		if got := buf.String(); got != want {
			t.Errorf("expected clearBytes + 3 clearPrevLine sequences (4 rows total) for 35 cols at width 10, got %q want %q", got, want)
		}
	})

	t.Run("stale content an exact multiple of the width does not over-count", func(t *testing.T) {
		buf := &bytes.Buffer{}
		exact := bytes.Repeat([]byte("x"), 80)
		a := &awareClearerDrawer{width: 40, visible: exact}
		st := &spinnerState{wrapped: buf, needClear: true}
		if err := a.clearMess(st); err != nil {
			t.Fatalf("clearMess: %v", err)
		}
		want := string(clearBytes) + string(clearPrevLine)
		if got := buf.String(); got != want {
			t.Errorf("expected clearBytes + 1 clearPrevLine sequence (2 rows total) for 80 cols at width 40, got %q want %q", got, want)
		}
	})

	t.Run("propagates write error", func(t *testing.T) {
		a := &awareClearerDrawer{width: 40, visible: []byte("x")}
		st := &spinnerState{wrapped: errWriter{err: errors.New("boom")}, needClear: true}
		if err := a.clearMess(st); err == nil {
			t.Error("expected clearMess to propagate the underlying write error")
		}
	})
}

func TestAwareClearerDrawer_HandleResize(t *testing.T) {
	t.Run("unchanged width is a no-op", func(t *testing.T) {
		buf := &bytes.Buffer{}
		calls := 0
		a := &awareClearerDrawer{
			width:    40,
			visible:  []byte("stale"),
			getWidth: func() int { calls++; return 40 },
		}
		st := &spinnerState{wrapped: buf, needClear: true, frame: []byte("stale")}

		if err := a.handleResize(st); err != nil {
			t.Fatalf("handleResize: %v", err)
		}
		if calls != 1 {
			t.Errorf("expected getWidth to be called exactly once, called %d times", calls)
		}
		if buf.Len() != 0 {
			t.Errorf("expected no write when width didn't change, got %q", buf.String())
		}
		if a.width != 40 {
			t.Errorf("expected width to stay unchanged, got %d", a.width)
		}
	})

	t.Run("changed width with nothing to clear still refreshes width and visible", func(t *testing.T) {
		a := &awareClearerDrawer{
			width:    5,
			visible:  []byte("short"),
			getWidth: func() int { return 80 },
		}
		st := &spinnerState{wrapped: &bytes.Buffer{}, needClear: false, frame: []byte("short")}

		if err := a.handleResize(st); err != nil {
			t.Fatalf("handleResize: %v", err)
		}
		if a.width != 80 {
			t.Errorf("expected width to update to 80, got %d", a.width)
		}
		if !bytes.Equal(a.visible, []byte("short")) {
			t.Errorf("expected visible to be recomputed from frame, got %q", a.visible)
		}
	})

	t.Run("changed width with something to clear writes then recomputes", func(t *testing.T) {
		buf := &bytes.Buffer{}
		a := &awareClearerDrawer{
			width:    10,
			visible:  []byte("stale-visible-content"),
			getWidth: func() int { return 5 },
		}
		st := &spinnerState{wrapped: buf, needClear: true, frame: []byte("new")}

		if err := a.handleResize(st); err != nil {
			t.Fatalf("handleResize: %v", err)
		}
		if buf.Len() == 0 {
			t.Error("expected a clear sequence to be written for the stale content")
		}
		if st.needClear {
			t.Error("expected needClear to be cleared")
		}
		if !bytes.Equal(a.visible, []byte("new")) {
			t.Errorf("expected visible to be recomputed from the new frame at the new width, got %q", a.visible)
		}
	})

	t.Run("write failure stops before recomputing visible", func(t *testing.T) {
		staleVisible := []byte("stale")
		a := &awareClearerDrawer{
			width:    10,
			visible:  staleVisible,
			getWidth: func() int { return 5 },
		}
		st := &spinnerState{wrapped: errWriter{err: errors.New("boom")}, needClear: true, frame: []byte("new")}

		if err := a.handleResize(st); err == nil {
			t.Fatal("expected handleResize to propagate the write error")
		}
		if !bytes.Equal(a.visible, staleVisible) {
			t.Errorf("expected visible to be left untouched after a failed clear, got %q", a.visible)
		}
	})
}

func TestAwareClearerDrawer_Clear(t *testing.T) {
	t.Run("unchanged width, ordinary clear", func(t *testing.T) {
		buf := &bytes.Buffer{}
		a := &awareClearerDrawer{width: 40, getWidth: func() int { return 40 }}
		st := &spinnerState{wrapped: buf, needClear: true}

		if err := a.clear(st); err != nil {
			t.Fatalf("clear: %v", err)
		}
		if got := buf.String(); got != string(clearBytes) {
			t.Errorf("expected a plain clearBytes write, got %q", got)
		}
		if st.needClear {
			t.Error("expected needClear to be false after clear")
		}
	})

	t.Run("unchanged width, nothing to clear", func(t *testing.T) {
		buf := &bytes.Buffer{}
		a := &awareClearerDrawer{width: 40, getWidth: func() int { return 40 }}
		st := &spinnerState{wrapped: buf, needClear: false}

		if err := a.clear(st); err != nil {
			t.Fatalf("clear: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("expected no write, got %q", buf.String())
		}
	})

	t.Run("changed width does not double-clear", func(t *testing.T) {
		buf := &bytes.Buffer{}
		a := &awareClearerDrawer{
			width:    20,
			visible:  []byte("short"),
			getWidth: func() int { return 40 },
		}
		st := &spinnerState{wrapped: buf, needClear: true, frame: []byte("short")}

		if err := a.clear(st); err != nil {
			t.Fatalf("clear: %v", err)
		}
		if got := buf.String(); got != string(clearBytes) {
			t.Errorf("expected exactly one clearBytes sequence (no double clear), got %q", got)
		}
	})

	t.Run("resize write failure stops before the ordinary needClear write", func(t *testing.T) {
		w := &failOnCallWriter{on: 1, err: errors.New("boom")}
		a := &awareClearerDrawer{
			width:    20,
			visible:  []byte("short"),
			getWidth: func() int { return 40 },
		}
		st := &spinnerState{wrapped: w, needClear: true, frame: []byte("short")}

		if err := a.clear(st); err == nil {
			t.Fatal("expected clear to propagate the resize-triggered write error")
		}
		if w.call != 1 {
			t.Errorf("expected exactly one write attempt, got %d", w.call)
		}
	})
}

func TestAwareClearerDrawer_Adjust(t *testing.T) {
	t.Run("fits via raw byte length fast path", func(t *testing.T) {
		a := &awareClearerDrawer{width: 40}
		st := &spinnerState{frame: []byte("short")}
		a.adjust(st)
		if !bytes.Equal(a.visible, st.frame) {
			t.Errorf("expected visible to equal frame unchanged, got %q", a.visible)
		}
	})

	t.Run("ANSI-styled content that fits still renders correctly", func(t *testing.T) {
		frame := []byte("\033[32mHi\033[0m")
		a := &awareClearerDrawer{width: 2}
		st := &spinnerState{frame: frame}
		a.adjust(st)
		if !bytes.Equal(a.visible, frame) {
			t.Errorf("expected visible to equal frame unchanged when display width fits, got %q", a.visible)
		}
	})

	t.Run("crops to the grapheme boundary when display width exceeds budget", func(t *testing.T) {
		frame := []byte("⠋⠙⠹⠸⠼")
		a := &awareClearerDrawer{width: 3}
		st := &spinnerState{frame: frame}
		a.adjust(st)
		want := "⠋⠙⠹"
		if string(a.visible) != want {
			t.Errorf("expected the first 3 glyphs to survive cropping, got %q want %q", a.visible, want)
		}
	})

	t.Run("passes ANSI sequences through even past the visible cutoff", func(t *testing.T) {
		frame := []byte("\033[32mxxxxx\033[0m")
		a := &awareClearerDrawer{width: 3}
		st := &spinnerState{frame: frame}
		a.adjust(st)
		want := "\033[32mxxx\033[0m"
		if string(a.visible) != want {
			t.Errorf("expected color open/close to survive truncation, got %q want %q", a.visible, want)
		}
	})

	t.Run("zero width drops all printable content but not ANSI", func(t *testing.T) {
		a := &awareClearerDrawer{width: 0}
		st := &spinnerState{frame: []byte("hi")}
		a.adjust(st)
		if len(a.visible) != 0 {
			t.Errorf("expected no visible content at width 0, got %q", a.visible)
		}
	})

	t.Run("display-width fast path aliases the frame instead of copying it", func(t *testing.T) {
		frame := []byte("⠋⠙⠹")
		a := &awareClearerDrawer{width: 3}
		st := &spinnerState{frame: frame}
		a.adjust(st)
		if !bytes.Equal(a.visible, frame) {
			t.Fatalf("expected visible to equal frame, got %q", a.visible)
		}

		frame[3] = 'X'
		if a.visible[3] != 'X' {
			t.Errorf("expected visible to alias frame's backing array (fast path taken), but it didn't observe the in-place mutation - got %q", a.visible)
		}
	})

	t.Run("display-width fast path should apply to ANSI-styled frames too", func(t *testing.T) {
		frame := []byte("\033[32mHi\033[0m")
		a := &awareClearerDrawer{width: 2}
		st := &spinnerState{frame: frame}
		a.adjust(st)
		if !bytes.Equal(a.visible, frame) {
			t.Fatalf("expected visible to equal frame, got %q", a.visible)
		}

		frame[5] = 'X'
		if a.visible[5] != 'X' {
			t.Errorf("expected the fast path to fire (aliasing frame) for an ANSI-styled frame that fits, but it took the slow (copying) path instead - got %q", a.visible)
		}
	})
}
