// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/creack/pty"
)

func openTestPTY(t *testing.T) *os.File {
	t.Helper()
	master, tty, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open: %v (platform without PTY support?)", err)
	}
	t.Cleanup(func() {
		_ = tty.Close()
		_ = master.Close()
	})
	return tty
}

func TestWrapPair_Errors(t *testing.T) {
	frame := staticFrame([]byte("*"))
	ticker := make(chan time.Time)

	t.Run("both writers nil", func(t *testing.T) {
		if _, err := WrapPair(context.Background(), nil, nil, frame, ticker); err == nil {
			t.Error("expected error when both writers are nil")
		}
	})

	t.Run("nil frame func", func(t *testing.T) {
		if _, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, nil, ticker); err == nil {
			t.Error("expected error when frame func is nil")
		}
	})

	t.Run("nil ticker", func(t *testing.T) {
		if _, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, frame, nil); err == nil {
			t.Error("expected error when ticker is nil")
		}
	})
}

func TestWrapPair_NilContextDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("WrapPair(nil, ...) panicked: %v", r)
		}
	}()
	pair, err := WrapPair(nil, &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time)) //nolint:staticcheck
	if err == nil && pair != nil {
		_ = pair.Spinner.Close()
	}
}

func TestWrapPair_SharesState(t *testing.T) {
	main := &syncBuffer{}
	spinner := &syncBuffer{}

	pair, err := WrapPair(context.Background(), main, spinner, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if asReal(t, pair.Standard).wrapped != main {
		t.Error("Standard.wrapped should be the main writer")
	}
	if asReal(t, pair.Spinner).wrapped != spinner {
		t.Error("Spinner.wrapped should be the spinner writer")
	}
	if asReal(t, pair.Standard).st != asReal(t, pair.Spinner).st {
		t.Error("Standard and Spinner should share the same spinner state")
	}
}

func TestWrapPair_MainFallsBackToSpinner(t *testing.T) {
	spinner := &syncBuffer{}

	pair, err := WrapPair(context.Background(), nil, spinner, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if asReal(t, pair.Standard).wrapped != spinner {
		t.Error("expected Standard writer to fall back to the spinner writer when main is nil")
	}
}

func TestWrapPair_SpinnerFallsBackToMain(t *testing.T) {
	main := &syncBuffer{}

	pair, err := WrapPair(context.Background(), main, nil, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if asReal(t, pair.Spinner).wrapped != main {
		t.Error("expected Spinner writer to fall back to the main writer when spinner is nil")
	}
}

func TestDefaultWrapOptions_FieldsAreSane(t *testing.T) {
	opt := DefaultWrapOptions()

	if opt.GetWidth != nil {
		t.Errorf("expected a nil default GetWidth (resize detection off), got %v", opt.GetWidth)
	}
}

func TestWrapWithResizeDetection_SetsGetWidth(t *testing.T) {
	getWidth := func() int { return 42 }

	opt := WrapWithResizeDetection(getWidth)(WrapOptions{})

	if opt.GetWidth == nil || opt.GetWidth() != 42 {
		t.Error("expected GetWidth to be set to the provided function")
	}
}

func TestWrapWithResizeDetection_NilGetWidthIsANoop(t *testing.T) {
	opt := WrapWithResizeDetection(nil)(WrapOptions{})
	if opt.GetWidth != nil {
		t.Errorf("expected a no-op, got %+v", opt)
	}
}

func TestWrapPair_ResizeDetectionOption_UsesAwareClearerDrawer(t *testing.T) {
	getWidth := func() int { return 42 }

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time), WrapWithResizeDetection(getWidth))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	if _, ok := asReal(t, pair.Spinner).st.cd.(*awareClearerDrawer); !ok {
		t.Errorf("expected an *awareClearerDrawer when GetWidth is set, got %T", asReal(t, pair.Spinner).st.cd)
	}
}

func TestWrapPair_NilOptionsFuncInSliceIsSkippedWithoutPanic(t *testing.T) {
	getWidth := func() int { return 42 }

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time), nil, WrapWithResizeDetection(getWidth), nil)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	if _, ok := asReal(t, pair.Spinner).st.cd.(*awareClearerDrawer); !ok {
		t.Errorf("expected a nil WrapOptionsFunc to be skipped and the real option after it still applied, got %T", asReal(t, pair.Spinner).st.cd)
	}
}

func TestWrapPair_ResizeDetectionOption_CallsGetWidthDirectlyWithNoImplicitCaching(t *testing.T) {
	width := &atomic.Int64{}
	width.Store(40)
	getWidth := func() int { return int(width.Load()) }

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time), WrapWithResizeDetection(getWidth))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	aware, ok := asReal(t, pair.Spinner).st.cd.(*awareClearerDrawer)
	if !ok {
		t.Fatalf("expected *awareClearerDrawer, got %T", asReal(t, pair.Spinner).st.cd)
	}
	if got := aware.getWidth(); got != 40 {
		t.Fatalf("expected initial width 40, got %d", got)
	}

	width.Store(80)
	if got := aware.getWidth(); got != 80 {
		t.Errorf("expected WrapPair to call the provided getWidth directly with no caching of its own (caching is now the caller's job via CachedGetWidth), got %d", got)
	}
}

func TestWrapPair_NoResizeDetectionOption_StaysOblivious(t *testing.T) {
	t.Run("nil GetWidth stays oblivious", func(t *testing.T) {
		pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time), WrapWithResizeDetection(nil))
		if err != nil {
			t.Fatalf("WrapPair: %v", err)
		}
		defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

		if _, ok := asReal(t, pair.Spinner).st.cd.(obliviousClearerDrawer); !ok {
			t.Errorf("expected an obliviousClearerDrawer when GetWidth is nil, got %T", asReal(t, pair.Spinner).st.cd)
		}
	})

	t.Run("no option set", func(t *testing.T) {
		pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
		if err != nil {
			t.Fatalf("WrapPair: %v", err)
		}
		defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

		if _, ok := asReal(t, pair.Spinner).st.cd.(obliviousClearerDrawer); !ok {
			t.Errorf("expected an obliviousClearerDrawer by default, got %T", asReal(t, pair.Spinner).st.cd)
		}
	})
}

func TestWrapFilePair_BothFilesNilErrors(t *testing.T) {
	if _, err := WrapFilePair(context.Background(), nil, nil, staticFrame([]byte("*")), make(chan time.Time)); err == nil {
		t.Error("expected an error when both main and spinner are nil")
	}
}

func TestWrapFilePair_NilMainFallsBackToSpinner(t *testing.T) {
	spinner := openTestPTY(t)

	pair, err := WrapFilePair(context.Background(), nil, spinner, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapFilePair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	if _, ok := pair.Standard.(stdWriter); !ok {
		t.Errorf("expected Standard to fall back to spinner (a real terminal) when main is nil, got %T", pair.Standard)
	}
}

func TestWrapFilePair_NilSpinnerFallsBackToMain(t *testing.T) {
	main := openTestPTY(t)

	pair, err := WrapFilePair(context.Background(), main, nil, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapFilePair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	if _, ok := pair.Spinner.(writerReal); !ok {
		t.Errorf("expected Spinner to fall back to main (a real terminal) when spinner is nil, got %T", pair.Spinner)
	}
}

func TestWrapFilePair_NilFrameFuncErrors(t *testing.T) {
	if _, err := WrapFilePair(context.Background(), os.Stdout, os.Stderr, nil, make(chan time.Time)); err == nil {
		t.Error("expected an error for a nil FrameFunc")
	}
}

func TestWrapFilePair_NilTickerErrors(t *testing.T) {
	if _, err := WrapFilePair(context.Background(), os.Stdout, os.Stderr, staticFrame([]byte("*")), nil); err == nil {
		t.Error("expected an error for a nil ticker")
	}
}

func TestWrapFilePair_ClosedSpinnerFallsBackToPassthrough(t *testing.T) {
	spinner, err := os.CreateTemp(t.TempDir(), "spinner")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = spinner.Close()

	pair, err := WrapFilePair(context.Background(), os.Stdout, spinner, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapFilePair: %v", err)
	}
	if _, ok := pair.Spinner.(WriterPassthrough); !ok {
		t.Errorf("expected Spinner to fall back to a passthrough writer for a closed fd, got %T", pair.Spinner)
	}
}

func TestWrapFilePair_ClosedMainFallsBackToPassthroughForStandardOnly(t *testing.T) {
	spinner := openTestPTY(t)

	main, err := os.CreateTemp(t.TempDir(), "main")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = main.Close()

	pair, err := WrapFilePair(context.Background(), main, spinner, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapFilePair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	if _, ok := pair.Standard.(WriterPassthrough); !ok {
		t.Errorf("expected Standard to fall back to a passthrough writer for a closed fd, got %T", pair.Standard)
	}
	if _, ok := pair.Spinner.(writerReal); !ok {
		t.Errorf("expected Spinner to stay a real spinner writer when it is a terminal, got %T", pair.Spinner)
	}
}

func TestWrapFilePair_SpinnerNotTerminalDisablesBoth(t *testing.T) {
	main, err := os.CreateTemp(t.TempDir(), "main")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = main.Close() }()

	spinner, err := os.CreateTemp(t.TempDir(), "spinner")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = spinner.Close() }()

	pair, err := WrapFilePair(context.Background(), main, spinner, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapFilePair: %v", err)
	}

	if standard, ok := pair.Standard.(WriterPassthrough); !ok {
		t.Errorf("expected Standard to be a passthrough writer when spinner is not a terminal, got %T", pair.Standard)
	} else if standard.Writer != main {
		t.Error("expected Standard's passthrough to wrap main")
	}
	if spinnerW, ok := pair.Spinner.(WriterPassthrough); !ok {
		t.Errorf("expected Spinner to be a passthrough writer when spinner is not a terminal, got %T", pair.Spinner)
	} else if spinnerW.Writer != spinner {
		t.Error("expected Spinner's passthrough to wrap spinner")
	}
}

func TestWrapFilePair_SpinnerNotTerminal_GetWidthReportsNothingToDetect(t *testing.T) {
	main, err := os.CreateTemp(t.TempDir(), "main")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = main.Close() }()

	spinner, err := os.CreateTemp(t.TempDir(), "spinner")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = spinner.Close() }()

	getWidth := func() int { return 123 }
	pair, err := WrapFilePair(context.Background(), main, spinner, staticFrame([]byte("*")), make(chan time.Time), WrapWithResizeDetection(getWidth))
	if err != nil {
		t.Fatalf("WrapFilePair: %v", err)
	}

	if got := pair.Spinner.GetWidth()(); got != -1 {
		t.Errorf("expected pair.Spinner.GetWidth() to report -1 (nothing to detect) once the Pair fell back "+
			"to a passthrough, regardless of what was configured, got %d instead", got)
	}
}

func TestWrapFilePair_MainNotTerminalDisablesOnlyStandard(t *testing.T) {
	spinnerTTY := openTestPTY(t)

	main, err := os.CreateTemp(t.TempDir(), "main")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = main.Close() }()

	pair, err := WrapFilePair(context.Background(), main, spinnerTTY, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapFilePair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	if standard, ok := pair.Standard.(WriterPassthrough); !ok {
		t.Errorf("expected Standard to be a passthrough writer when main is not a terminal, got %T", pair.Standard)
	} else if standard.Writer != main {
		t.Error("expected Standard's passthrough to wrap main")
	}
	if _, ok := pair.Spinner.(writerReal); !ok {
		t.Errorf("expected Spinner to stay a real spinner writer when it is a terminal, got %T", pair.Spinner)
	}
}

func TestWrapFilePair_BothTerminalsKeepsBothReal(t *testing.T) {
	mainTTY := openTestPTY(t)
	spinnerTTY := openTestPTY(t)

	pair, err := WrapFilePair(context.Background(), mainTTY, spinnerTTY, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapFilePair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	if _, ok := pair.Standard.(stdWriter); !ok {
		t.Errorf("expected Standard to stay a real spinner writer when both streams are terminals, got %T", pair.Standard)
	}
	if _, ok := pair.Spinner.(writerReal); !ok {
		t.Errorf("expected Spinner to stay a real spinner writer when both streams are terminals, got %T", pair.Spinner)
	}
}

func TestWrapOS_NilFrameFuncErrors(t *testing.T) {
	if _, err := WrapOS(context.Background(), nil, make(chan time.Time)); err == nil {
		t.Error("expected an error for a nil FrameFunc")
	}
}

func TestWrapOS_NilTickerErrors(t *testing.T) {
	if _, err := WrapOS(context.Background(), staticFrame([]byte("*")), nil); err == nil {
		t.Error("expected an error for a nil ticker")
	}
}

func TestWrapOS_CIEnvDisablesSpinner(t *testing.T) {
	t.Setenv("CI", "1")

	pair, err := WrapOS(context.Background(), staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapOS: %v", err)
	}

	if _, ok := pair.Standard.(WriterPassthrough); !ok {
		t.Errorf("expected Standard to be a passthrough writer under CI, got %T", pair.Standard)
	}
	if _, ok := pair.Spinner.(WriterPassthrough); !ok {
		t.Errorf("expected Spinner to be a passthrough writer under CI, got %T", pair.Spinner)
	}
}

func TestWrapOS_CIEnv_GetWidthReportsNothingToDetect(t *testing.T) {
	t.Setenv("CI", "1")

	getWidth := func() int { return 123 }
	pair, err := WrapOS(context.Background(), staticFrame([]byte("*")), make(chan time.Time), WrapWithResizeDetection(getWidth))
	if err != nil {
		t.Fatalf("WrapOS: %v", err)
	}

	if got := pair.Spinner.GetWidth()(); got != -1 {
		t.Errorf("expected pair.Spinner.GetWidth() to report -1 (nothing to detect) under CI, got %d instead", got)
	}
}

func TestResizeAwareGetWidthPanicHelperProcess(t *testing.T) {
	if os.Getenv("SPINQ_RESIZE_GETWIDTH_PANIC_HELPER") != "1" {
		t.Skip("only runs as a subprocess helper; see TestWrapWithResizeDetection_PanickingGetWidthDoesNotCrashProcess")
	}

	panicky := func() int { panic("boom: deliberate getWidth panic") }
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time), WrapWithResizeDetection(panicky))
	if err != nil {
		fmt.Fprintln(os.Stderr, "WrapPair:", err)
		os.Exit(1)
	}

	if err := pair.Spinner.Start(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "Start:", err)
		os.Exit(1)
	}
	_, _ = pair.Standard.Write([]byte("trigger a clear/draw cycle\n"))
	_ = pair.Spinner.Close()

	fmt.Println("SURVIVED")
}

func TestWrapWithResizeDetection_PanickingGetWidthDoesNotCrashProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestResizeAwareGetWidthPanicHelperProcess$", "-test.v")
	cmd.Env = append(os.Environ(), "SPINQ_RESIZE_GETWIDTH_PANIC_HELPER=1")
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("subprocess did not exit within the timeout, output:\n%s", out)
	}
	if err != nil {
		t.Errorf("expected a panicking getWidth wired via WrapWithResizeDetection not to crash the process: %v\noutput:\n%s", err, out)
		return
	}
	if !strings.Contains(string(out), "SURVIVED") {
		t.Errorf("expected the process to survive a panicking resize-detection getWidth, output:\n%s", out)
	}
}
