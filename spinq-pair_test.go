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
)

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

func TestWrapFilePair_SpinnyStatErrorPropagates(t *testing.T) {
	spinny, err := os.CreateTemp(t.TempDir(), "spinny")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = spinny.Close()

	if _, err := WrapFilePair(context.Background(), os.Stdout, spinny, staticFrame([]byte("*")), make(chan time.Time)); err == nil {
		t.Error("expected an error when spinny.Stat() fails")
	}
}

func TestWrapFilePair_MainStatErrorPropagates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX character device (/dev/null)")
	}

	spinny, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer func() { _ = spinny.Close() }()

	main, err := os.CreateTemp(t.TempDir(), "main")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = main.Close()

	if _, err := WrapFilePair(context.Background(), main, spinny, staticFrame([]byte("*")), make(chan time.Time)); err == nil {
		t.Error("expected an error when main.Stat() fails")
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
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer func() { _ = devNull.Close() }()

	main, err := os.CreateTemp(t.TempDir(), "main")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = main.Close() }()

	pair, err := WrapFilePair(context.Background(), main, devNull, staticFrame([]byte("*")), make(chan time.Time))
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
	mainNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer func() { _ = mainNull.Close() }()

	spinnyNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer func() { _ = spinnyNull.Close() }()

	pair, err := WrapFilePair(context.Background(), mainNull, spinnyNull, staticFrame([]byte("*")), make(chan time.Time))
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
