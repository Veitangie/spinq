// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testCtxKey struct{}

func TestEvery_TicksAtRoughlyTheGivenInterval(t *testing.T) {
	ticker := Every(10 * time.Millisecond)
	select {
	case <-ticker:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Every's channel never ticked")
	}
}

func TestDefault_FieldsAreSane(t *testing.T) {
	opt := Default()

	if opt.Context != context.Background() {
		t.Errorf("expected Context to be context.Background(), got %v", opt.Context)
	}
	if opt.StartContext != nil {
		t.Errorf("expected StartContext to be nil (meaning: inherit from Context), got %v", opt.StartContext)
	}
	if opt.Ticker == nil {
		t.Fatal("expected a non-nil default Ticker")
	}
	select {
	case <-opt.Ticker:
	case <-time.After(500 * time.Millisecond):
		t.Error("default Ticker never ticked within 500ms of its 100ms interval")
	}
	if opt.Frame != nil {
		t.Error("expected a nil default Frame (built lazily by JustStart)")
	}
	if opt.Text != "Running" {
		t.Errorf("expected default Text %q, got %q", "Running", opt.Text)
	}
	if !slices.Equal(opt.States, DotsStates) {
		t.Errorf("expected default States to be DotsStates, got %v", opt.States)
	}
}

func TestJustStart_DefaultTemplateComposesAsExpected(t *testing.T) {
	opt := Default()
	frame := Join(opt.Divider, Surrounded(" ", Simple(opt.States), fmt.Sprintf(" %s (", opt.Text)), Duration(time.Now), Static(")"))

	got, err := frame()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(got)
	if !strings.Contains(s, "Running (") || !strings.HasSuffix(s, ")") {
		t.Errorf("expected the default frame to read like %q, got %q", " ⠋ Running (0.0s)", s)
	}
}

func TestJustStart_CustomizedTemplateComposesAsExpected(t *testing.T) {
	opt := Default()
	opt = WithText("Uploading")(opt)
	opt = WithStates([]string{"a", "b"})(opt)
	opt = WithDivider(" | ")(opt)

	frame := Join(opt.Divider, Surrounded(" ", Simple(opt.States), fmt.Sprintf(" %s (", opt.Text)), Duration(time.Now), Static(")"))

	got, err := frame()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(got)
	if !strings.Contains(s, " a Uploading (") || !strings.Contains(s, " | ") {
		t.Errorf("expected the customized template to reflect Text/States/Divider, got %q", s)
	}
}

func TestWithContext_SetsContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), testCtxKey{}, "marker")
	opt := WithContext(ctx)(JustStartOptions{})
	if opt.Context != ctx {
		t.Errorf("expected Context to be set to the given ctx")
	}
}

func TestWithContext_NilIsANoOp(t *testing.T) {
	original := context.WithValue(context.Background(), testCtxKey{}, "marker")
	opt := WithContext(original)(JustStartOptions{})
	opt = WithContext(nil)(opt) //nolint:staticcheck
	if opt.Context != original {
		t.Errorf("expected WithContext(nil) to leave the previous Context untouched, got %v", opt.Context)
	}
}

func TestWithStartContext_SetsStartContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), testCtxKey{}, "marker")
	opt := WithStartContext(ctx)(JustStartOptions{})
	if opt.StartContext != ctx {
		t.Errorf("expected StartContext to be set to the given ctx")
	}
}

func TestWithStartContext_NilActivelyResets(t *testing.T) {
	ctx := context.WithValue(context.Background(), testCtxKey{}, "marker")
	opt := WithStartContext(ctx)(JustStartOptions{})
	opt = WithStartContext(nil)(opt) //nolint:staticcheck
	if opt.StartContext != nil {
		t.Errorf("expected WithStartContext(nil) to reset StartContext to nil, got %v", opt.StartContext)
	}
}

func TestWithTicker_SetsTicker(t *testing.T) {
	ticker := make(chan time.Time)
	opt := WithTicker(ticker)(JustStartOptions{})
	if opt.Ticker != (<-chan time.Time)(ticker) {
		t.Errorf("expected Ticker to be set to the given channel")
	}
}

func TestWithTicker_NilIsANoOp(t *testing.T) {
	original := make(chan time.Time)
	opt := WithTicker(original)(JustStartOptions{})
	opt = WithTicker(nil)(opt)
	if opt.Ticker != (<-chan time.Time)(original) {
		t.Errorf("expected WithTicker(nil) to leave the previous Ticker untouched")
	}
}

func TestWithDuration_SetsATickingTicker(t *testing.T) {
	opt := WithDuration(10 * time.Millisecond)(JustStartOptions{})
	if opt.Ticker == nil {
		t.Fatal("expected WithDuration to set a non-nil Ticker")
	}
	select {
	case <-opt.Ticker:
	case <-time.After(500 * time.Millisecond):
		t.Error("Ticker from WithDuration never ticked")
	}
}

func TestWithFrame_SetsFrame(t *testing.T) {
	f := Static("custom")
	opt := WithFrame(f)(JustStartOptions{})
	got, err := opt.Frame()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "custom" {
		t.Errorf("got %q, want %q", got, "custom")
	}
}

func TestWithFrame_NilIsANoOp(t *testing.T) {
	original := Static("original")
	opt := WithFrame(original)(JustStartOptions{})
	opt = WithFrame(nil)(opt)
	got, err := opt.Frame()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "original" {
		t.Errorf("expected WithFrame(nil) to leave the previous Frame untouched, got %q", got)
	}
}

func TestWithText_SetsText(t *testing.T) {
	opt := WithText("Uploading")(JustStartOptions{})
	if opt.Text != "Uploading" {
		t.Errorf("got %q, want %q", opt.Text, "Uploading")
	}
}

func TestWithStates_SetsStates(t *testing.T) {
	states := []string{"x", "y", "z"}
	opt := WithStates(states)(JustStartOptions{})
	if !slices.Equal(opt.States, states) {
		t.Errorf("got %v, want %v", opt.States, states)
	}
}

func TestWithDivider_SetsDivider(t *testing.T) {
	opt := WithDivider(" | ")(JustStartOptions{})
	if opt.Divider != " | " {
		t.Errorf("got %q, want %q", opt.Divider, " | ")
	}
}

func TestWithResizeDetection_SetsSigwinChAndGetWidth(t *testing.T) {
	sigwinCh := make(chan struct{})
	getWidth := func() int { return 42 }

	opt := WithResizeDetection(sigwinCh, getWidth)(JustStartOptions{})

	if opt.SigwinCh == nil {
		t.Error("expected SigwinCh to be set")
	}
	if opt.GetWidth == nil || opt.GetWidth() != 42 {
		t.Error("expected GetWidth to be set to the provided function")
	}
}

func TestWithResizeDetection_NilArgsAreANoop(t *testing.T) {
	sigwinCh := make(chan struct{})
	getWidth := func() int { return 42 }

	t.Run("nil sigwinCh", func(t *testing.T) {
		opt := WithResizeDetection(nil, getWidth)(JustStartOptions{})
		if opt.SigwinCh != nil || opt.GetWidth != nil {
			t.Errorf("expected a no-op, got %+v", opt)
		}
	})

	t.Run("nil getWidth", func(t *testing.T) {
		opt := WithResizeDetection(sigwinCh, nil)(JustStartOptions{})
		if opt.SigwinCh != nil || opt.GetWidth != nil {
			t.Errorf("expected a no-op, got %+v", opt)
		}
	})
}

func TestWithUnmanagedResizeDetection_SetsGetWidthClearsSigwinCh(t *testing.T) {
	getWidth := func() int { return 42 }

	opt := WithUnmanagedResizeDetection(getWidth)(JustStartOptions{SigwinCh: make(chan struct{})})

	if opt.SigwinCh != nil {
		t.Error("expected SigwinCh to be cleared")
	}
	if opt.GetWidth == nil || opt.GetWidth() != 42 {
		t.Error("expected GetWidth to be set to the provided function")
	}
}

func TestWithUnmanagedResizeDetection_NilGetWidthIsANoop(t *testing.T) {
	existingSigwinCh := make(chan struct{})
	opt := WithUnmanagedResizeDetection(nil)(JustStartOptions{SigwinCh: existingSigwinCh})

	if opt.SigwinCh != (<-chan struct{})(existingSigwinCh) {
		t.Error("expected an existing SigwinCh to be left untouched by a nil getWidth")
	}
	if opt.GetWidth != nil {
		t.Error("expected GetWidth to stay unset")
	}
}

func TestWithJustStartOptions_ReplacesWholeStruct(t *testing.T) {
	replacement := JustStartOptions{
		Context: context.WithValue(context.Background(), testCtxKey{}, "replacement"),
		Frame:   Static("replaced"),
	}

	opt := WithTicker(make(chan time.Time))(JustStartOptions{})
	opt = WithJustStartOptions(replacement)(opt)

	if opt.Context != replacement.Context {
		t.Errorf("expected Context to come from the replacement struct")
	}
	if opt.Ticker != nil {
		t.Errorf("expected the earlier WithTicker value to be discarded, got %v", opt.Ticker)
	}
	got, err := opt.Frame()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "replaced" {
		t.Errorf("got %q, want %q", got, "replaced")
	}
}

func TestJustStart_DefaultOptionsSucceedsViaPassthrough(t *testing.T) {
	pair, err := JustStart()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pair == nil {
		t.Fatal("expected a non-nil pair")
	}

	if err := pair.Spinny.Start(context.Background()); err != nil {
		t.Errorf("expected passthrough Start to be a no-op, got %v", err)
	}

	select {
	case _, ok := <-pair.Err():
		if ok {
			t.Errorf("expected a passthrough pair's Err() channel to be closed with no values")
		}
	default:
		t.Errorf("expected a passthrough pair's Err() channel to be immediately ready (closed)")
	}

	pair.Close()
}

func TestJustStart_OptionsAreApplied(t *testing.T) {
	pair, err := JustStart(WithFrame(Static("custom")), WithTicker(make(chan time.Time)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pair == nil {
		t.Fatal("expected a non-nil pair")
	}
	pair.Close()
}

func withCIUnset(t *testing.T) {
	t.Helper()
	orig, wasSet := os.LookupEnv("CI")
	_ = os.Unsetenv("CI")
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv("CI", orig)
		}
	})
}

func withRealTerminalStdio(t *testing.T) {
	t.Helper()
	withCIUnset(t)

	outTTY := openTestPTY(t)
	errTTY := openTestPTY(t)

	origStdout, origStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outTTY, errTTY
	t.Cleanup(func() { os.Stdout, os.Stderr = origStdout, origStderr })
}

func TestJustStart_ResizeDetectionRoutesToAwareClearerDrawer(t *testing.T) {
	t.Run("managed", func(t *testing.T) {
		withRealTerminalStdio(t)
		pair, err := JustStart(WithResizeDetection(make(chan struct{}), func() int { return 80 }))
		if err != nil {
			t.Fatalf("JustStart: %v", err)
		}
		defer callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

		if _, ok := asReal(t, pair.Spinny).st.cd.(*awareClearerDrawer); !ok {
			t.Errorf("expected an *awareClearerDrawer, got %T", asReal(t, pair.Spinny).st.cd)
		}
	})

	t.Run("managed uses live getWidth, not the raw one", func(t *testing.T) {
		withRealTerminalStdio(t)
		width := &atomic.Int64{}
		width.Store(40)
		pair, err := JustStart(WithResizeDetection(make(chan struct{}, 1), func() int { return int(width.Load()) }))
		if err != nil {
			t.Fatalf("JustStart: %v", err)
		}
		defer callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

		aware, ok := asReal(t, pair.Spinny).st.cd.(*awareClearerDrawer)
		if !ok {
			t.Fatalf("expected *awareClearerDrawer, got %T", asReal(t, pair.Spinny).st.cd)
		}
		if got := aware.getWidth(); got != 40 {
			t.Fatalf("expected initial width 40, got %d", got)
		}

		width.Store(80)
		if got := aware.getWidth(); got != 40 {
			t.Errorf("expected the managed getWidth to stay cached at 40 without a sigwinCh signal, got %d", got)
		}
	})

	t.Run("unmanaged", func(t *testing.T) {
		withRealTerminalStdio(t)
		pair, err := JustStart(WithUnmanagedResizeDetection(func() int { return 80 }))
		if err != nil {
			t.Fatalf("JustStart: %v", err)
		}
		defer callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

		if _, ok := asReal(t, pair.Spinny).st.cd.(*awareClearerDrawer); !ok {
			t.Errorf("expected an *awareClearerDrawer (unmanaged mode), got %T", asReal(t, pair.Spinny).st.cd)
		}
	})

	t.Run("neither configured stays oblivious", func(t *testing.T) {
		withRealTerminalStdio(t)
		pair, err := JustStart()
		if err != nil {
			t.Fatalf("JustStart: %v", err)
		}
		defer callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

		if _, ok := asReal(t, pair.Spinny).st.cd.(obliviousClearerDrawer); !ok {
			t.Errorf("expected an obliviousClearerDrawer by default, got %T", asReal(t, pair.Spinny).st.cd)
		}
	})
}
