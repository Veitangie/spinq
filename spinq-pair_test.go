// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import (
	"context"
	"errors"
	"os"
	"runtime"
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

func TestWrapPair_SharesState(t *testing.T) {
	main := &syncBuffer{}
	spinny := &syncBuffer{}

	pair, err := WrapPair(context.Background(), main, spinny, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if asReal(t, pair.Standard).wrapped != main {
		t.Error("Standard.wrapped should be the main writer")
	}
	if asReal(t, pair.Spinny).wrapped != spinny {
		t.Error("Spinny.wrapped should be the spinny writer")
	}
	if asReal(t, pair.Standard).st != asReal(t, pair.Spinny).st {
		t.Error("Standard and Spinny should share the same spinner state")
	}
}

func TestWrapPair_MainFallsBackToSpinny(t *testing.T) {
	spinny := &syncBuffer{}

	pair, err := WrapPair(context.Background(), nil, spinny, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if asReal(t, pair.Standard).wrapped != spinny {
		t.Error("expected Standard writer to fall back to the spinny writer when main is nil")
	}
}

func TestWrapPair_SpinnyFallsBackToMain(t *testing.T) {
	main := &syncBuffer{}

	pair, err := WrapPair(context.Background(), main, nil, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if asReal(t, pair.Spinny).wrapped != main {
		t.Error("expected Spinny writer to fall back to the main writer when spinny is nil")
	}
}

func TestWrapWithResizeDetection_SetsSigwinChAndGetWidth(t *testing.T) {
	sigwinCh := make(chan struct{})
	getWidth := func() int { return 42 }

	opt := WrapWithResizeDetection(sigwinCh, getWidth)(WrapOptions{})

	if opt.SigwinCh == nil {
		t.Error("expected SigwinCh to be set")
	}
	if opt.GetWidth == nil || opt.GetWidth() != 42 {
		t.Error("expected GetWidth to be set to the provided function")
	}
}

func TestWrapWithResizeDetection_NilArgsAreANoop(t *testing.T) {
	sigwinCh := make(chan struct{})
	getWidth := func() int { return 42 }

	t.Run("nil sigwinCh", func(t *testing.T) {
		opt := WrapWithResizeDetection(nil, getWidth)(WrapOptions{})
		if opt.SigwinCh != nil || opt.GetWidth != nil {
			t.Errorf("expected a no-op, got %+v", opt)
		}
	})

	t.Run("nil getWidth", func(t *testing.T) {
		opt := WrapWithResizeDetection(sigwinCh, nil)(WrapOptions{})
		if opt.SigwinCh != nil || opt.GetWidth != nil {
			t.Errorf("expected a no-op, got %+v", opt)
		}
	})
}

func TestWrapWithUnmanagedResizeDetection_SetsGetWidthClearsSigwinCh(t *testing.T) {
	getWidth := func() int { return 42 }

	opt := WrapWithUnmanagedResizeDetection(getWidth)(WrapOptions{SigwinCh: make(chan struct{})})

	if opt.SigwinCh != nil {
		t.Error("expected SigwinCh to be cleared")
	}
	if opt.GetWidth == nil || opt.GetWidth() != 42 {
		t.Error("expected GetWidth to be set to the provided function")
	}
}

func TestWrapWithUnmanagedResizeDetection_NilGetWidthIsANoop(t *testing.T) {
	existingSigwinCh := make(chan struct{})
	opt := WrapWithUnmanagedResizeDetection(nil)(WrapOptions{SigwinCh: existingSigwinCh})

	if opt.SigwinCh != (<-chan struct{})(existingSigwinCh) {
		t.Error("expected an existing SigwinCh to be left untouched by a nil getWidth")
	}
	if opt.GetWidth != nil {
		t.Error("expected GetWidth to stay unset")
	}
}

func TestWrapPair_ResizeDetectionOption_UsesAwareClearerDrawer(t *testing.T) {
	sigwinCh := make(chan struct{})
	getWidth := func() int { return 42 }

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time), WrapWithResizeDetection(sigwinCh, getWidth))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

	if _, ok := asReal(t, pair.Spinny).st.cd.(*awareClearerDrawer); !ok {
		t.Errorf("expected an *awareClearerDrawer when both SigwinCh and GetWidth are set, got %T", asReal(t, pair.Spinny).st.cd)
	}
}

func TestWrapPair_ResizeDetectionOption_UsesLiveGetWidthNotRawGetWidth(t *testing.T) {
	sigwinCh := make(chan struct{}, 1)
	width := &atomic.Int64{}
	width.Store(40)
	getWidth := func() int { return int(width.Load()) }

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time), WrapWithResizeDetection(sigwinCh, getWidth))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
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
		t.Errorf("expected the managed getWidth to stay cached at 40 without a sigwinCh signal (proving it's LiveGetWidth-wrapped, not the raw getWidth passed through directly), got %d", got)
	}
}

func TestWrapPair_PartialResizeDetectionOption(t *testing.T) {
	t.Run("only sigwinCh set stays oblivious", func(t *testing.T) {
		pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time), WrapWithResizeDetection(make(chan struct{}), nil))
		if err != nil {
			t.Fatalf("WrapPair: %v", err)
		}
		defer callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

		if _, ok := asReal(t, pair.Spinny).st.cd.(obliviousClearerDrawer); !ok {
			t.Errorf("expected an obliviousClearerDrawer when GetWidth is nil, got %T", asReal(t, pair.Spinny).st.cd)
		}
	})

	t.Run("only getWidth set uses unmanaged awareClearerDrawer", func(t *testing.T) {
		pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time), WrapWithUnmanagedResizeDetection(func() int { return 1 }))
		if err != nil {
			t.Fatalf("WrapPair: %v", err)
		}
		defer callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

		if _, ok := asReal(t, pair.Spinny).st.cd.(*awareClearerDrawer); !ok {
			t.Errorf("expected an *awareClearerDrawer (unmanaged mode) when only GetWidth is set, got %T", asReal(t, pair.Spinny).st.cd)
		}
	})

	t.Run("neither set", func(t *testing.T) {
		pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
		if err != nil {
			t.Fatalf("WrapPair: %v", err)
		}
		defer callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

		if _, ok := asReal(t, pair.Spinny).st.cd.(obliviousClearerDrawer); !ok {
			t.Errorf("expected an obliviousClearerDrawer by default, got %T", asReal(t, pair.Spinny).st.cd)
		}
	})
}

func TestWrapFilePair_BothFilesNilErrors(t *testing.T) {
	if _, err := WrapFilePair(context.Background(), nil, nil, staticFrame([]byte("*")), make(chan time.Time)); err == nil {
		t.Error("expected an error when both main and spinny are nil")
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

func TestWrapFilePair_ClosedSpinnyFallsBackToPassthrough(t *testing.T) {
	spinny, err := os.CreateTemp(t.TempDir(), "spinny")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = spinny.Close()

	pair, err := WrapFilePair(context.Background(), os.Stdout, spinny, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapFilePair: %v", err)
	}
	if _, ok := pair.Spinny.(SpinqWriterPassthrough); !ok {
		t.Errorf("expected Spinny to fall back to a passthrough writer for a closed fd, got %T", pair.Spinny)
	}
}

func TestWrapFilePair_ClosedMainFallsBackToPassthroughForStandardOnly(t *testing.T) {
	spinny := openTestPTY(t)

	main, err := os.CreateTemp(t.TempDir(), "main")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = main.Close()

	pair, err := WrapFilePair(context.Background(), main, spinny, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapFilePair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinny.Stop() })

	if _, ok := pair.Standard.(SpinqWriterPassthrough); !ok {
		t.Errorf("expected Standard to fall back to a passthrough writer for a closed fd, got %T", pair.Standard)
	}
	if _, ok := pair.Spinny.(SpinqWriterReal); !ok {
		t.Errorf("expected Spinny to stay a real spinner writer when it is a terminal, got %T", pair.Spinny)
	}
}

func TestWrapFilePair_SpinnyNotTerminalDisablesBoth(t *testing.T) {
	main, err := os.CreateTemp(t.TempDir(), "main")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = main.Close() }()

	spinny, err := os.CreateTemp(t.TempDir(), "spinny")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = spinny.Close() }()

	pair, err := WrapFilePair(context.Background(), main, spinny, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapFilePair: %v", err)
	}

	if standard, ok := pair.Standard.(SpinqWriterPassthrough); !ok {
		t.Errorf("expected Standard to be a passthrough writer when spinny is not a terminal, got %T", pair.Standard)
	} else if standard.Writer != main {
		t.Error("expected Standard's passthrough to wrap main")
	}
	if spinnyW, ok := pair.Spinny.(SpinqWriterPassthrough); !ok {
		t.Errorf("expected Spinny to be a passthrough writer when spinny is not a terminal, got %T", pair.Spinny)
	} else if spinnyW.Writer != spinny {
		t.Error("expected Spinny's passthrough to wrap spinny")
	}
}

func TestWrapFilePair_MainNotTerminalDisablesOnlyStandard(t *testing.T) {
	spinnyTTY := openTestPTY(t)

	main, err := os.CreateTemp(t.TempDir(), "main")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = main.Close() }()

	pair, err := WrapFilePair(context.Background(), main, spinnyTTY, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapFilePair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinny.Stop() })

	if standard, ok := pair.Standard.(SpinqWriterPassthrough); !ok {
		t.Errorf("expected Standard to be a passthrough writer when main is not a terminal, got %T", pair.Standard)
	} else if standard.Writer != main {
		t.Error("expected Standard's passthrough to wrap main")
	}
	if _, ok := pair.Spinny.(SpinqWriterReal); !ok {
		t.Errorf("expected Spinny to stay a real spinner writer when it is a terminal, got %T", pair.Spinny)
	}
}

func TestWrapFilePair_BothTerminalsKeepsBothReal(t *testing.T) {
	mainTTY := openTestPTY(t)
	spinnyTTY := openTestPTY(t)

	pair, err := WrapFilePair(context.Background(), mainTTY, spinnyTTY, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapFilePair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinny.Stop() })

	if _, ok := pair.Standard.(SpinqWriterReal); !ok {
		t.Errorf("expected Standard to stay a real spinner writer when both streams are terminals, got %T", pair.Standard)
	}
	if _, ok := pair.Spinny.(SpinqWriterReal); !ok {
		t.Errorf("expected Spinny to stay a real spinner writer when both streams are terminals, got %T", pair.Spinny)
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

	if _, ok := pair.Standard.(SpinqWriterPassthrough); !ok {
		t.Errorf("expected Standard to be a passthrough writer under CI, got %T", pair.Standard)
	}
	if _, ok := pair.Spinny.(SpinqWriterPassthrough); !ok {
		t.Errorf("expected Spinny to be a passthrough writer under CI, got %T", pair.Spinny)
	}
}

func TestClose_ClearsDisplay(t *testing.T) {
	spinny := &syncBuffer{}
	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

	if got := spinny.String(); !strings.HasSuffix(got, string(clearBytes)) {
		t.Errorf("expected Close to clear the display, got %q", got)
	}
}

func TestClose_SubsequentCallsReturnErrClosed(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Standard.Start(context.Background()) })
	if !errors.Is(err, ErrClosed) {
		t.Errorf("expected Start after Close to return ErrClosed, got %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Stop", func() { err = pair.Standard.Stop() })
	if !errors.Is(err, ErrClosed) {
		t.Errorf("expected Stop after Close to return ErrClosed, got %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Set", func() { err = pair.Standard.Set(staticFrame([]byte("x"))) })
	if !errors.Is(err, ErrClosed) {
		t.Errorf("expected Set after Close to return ErrClosed, got %v", err)
	}
}

func TestClose_DoesNotLeakInFlightTickFetch(t *testing.T) {
	spinny := &syncBuffer{}
	ticker := make(chan time.Time)
	proceed := make(chan struct{})
	var calls atomic.Int32
	frameFn := func() ([]byte, error) {
		if calls.Add(1) == 1 {
			return []byte("*"), nil
		}
		<-proceed
		return []byte("*"), nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, frameFn, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	ticker <- time.Now()
	waitForCondition(t, func() bool { return calls.Load() == 2 })

	callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

	stuck := runtime.NumGoroutine()
	close(proceed)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() < stuck {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("expected the in-flight tick fetch's goroutine to exit after Close instead of leaking forever — goroutine count never dropped below %d:\n%s", stuck, dumpGoroutines())
}

func TestErr_ReceivesErrorOnDrawFrameWriteFailure(t *testing.T) {
	writeErr := errors.New("draw boom")
	spinny := &failAfterWriter{n: 1, err: writeErr}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, Simple([]string{"a", "b"}), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

	ticker <- time.Now()

	select {
	case gotErr, ok := <-pair.Err():
		if !ok {
			t.Fatal("expected an error, got a closed channel with no value")
		}
		if !errors.Is(gotErr, writeErr) {
			t.Errorf("expected error wrapping %v, got %v", writeErr, gotErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected an error on Err() after the tick-triggered draw failed")
	}
}

func TestErr_ReceivesErrorOnWindDownClearFailure(t *testing.T) {
	writeErr := errors.New("clear boom")
	spinny := &failAfterWriter{n: 1, err: writeErr}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

	select {
	case gotErr, ok := <-pair.Err():
		if !ok {
			t.Fatal("expected an error buffered before the channel closed, got none")
		}
		if !errors.Is(gotErr, writeErr) {
			t.Errorf("expected error wrapping %v, got %v", writeErr, gotErr)
		}
	default:
		t.Fatal("expected an error to already be buffered in Err() immediately after Close returns")
	}
}

func TestErr_ChannelClosesAfterClose(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

	select {
	case _, ok := <-pair.Err():
		if ok {
			t.Error("expected no buffered error since nothing failed, but got one")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected Err() to be immediately readable (closed) after Close, but it blocked")
	}
}

func TestErr_BestEffortSendDoesNotBlockCloseWhenBufferIsFull(t *testing.T) {
	writeErr := errors.New("always boom")
	spinny := &failAfterWriter{n: 1, err: writeErr}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, Simple([]string{"a", "b"}), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	ticker <- time.Now()
	time.Sleep(100 * time.Millisecond)

	start := time.Now()
	callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("expected Close to complete quickly via the best-effort (~10ms) send even with a full, undrained Err() buffer from an earlier failure, took %s", elapsed)
	}
}

func TestWrite_AfterClose_PassesThroughWithoutResurrectingStaleFrame(t *testing.T) {
	shared := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), shared, shared, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForCondition(t, func() bool { return strings.Contains(shared.String(), "*") })

	callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

	afterClose := shared.String()
	if strings.HasSuffix(afterClose, "*") {
		t.Fatalf("frame character still visible right after Close, before any post-close write: %q", afterClose)
	}

	n, err := pair.Standard.Write([]byte("goodbye\n"))
	if err != nil {
		t.Errorf("expected Write after Close to succeed as a plain passthrough, got %v", err)
	}
	if n != len("goodbye\n") {
		t.Errorf("expected all %d bytes written, got %d", len("goodbye\n"), n)
	}

	final := shared.String()
	if final != afterClose+"goodbye\n" {
		t.Errorf("expected the post-Close write to reach the stream untouched; got %q", final)
	}
}
