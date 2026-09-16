// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWriterPassthrough_AllLifecycleMethodsAreNoOps(t *testing.T) {
	sw := WriterPassthrough{Writer: &syncBuffer{}}

	if err := sw.Start(context.Background()); err != nil {
		t.Errorf("Start: expected nil, got %v", err)
	}
	if err := sw.Stop(); err != nil {
		t.Errorf("Stop: expected nil, got %v", err)
	}
	if err := sw.StopWith("message"); err != nil {
		t.Errorf("StopWith: expected nil, got %v", err)
	}
	if err := sw.StopNoClear("suffix"); err != nil {
		t.Errorf("StopNoClear: expected nil, got %v", err)
	}
	if err := sw.SetFrame(staticFrame([]byte("x"))); err != nil {
		t.Errorf("SetFrame: expected nil, got %v", err)
	}
	if err := sw.SetFrameWith(staticFrame([]byte("x")), "message"); err != nil {
		t.Errorf("SetFrameWith: expected nil, got %v", err)
	}
	if err := sw.SetFrameNoClear(staticFrame([]byte("x")), "suffix"); err != nil {
		t.Errorf("SetFrameNoClear: expected nil, got %v", err)
	}
	if err := sw.SetTicker(make(chan time.Time)); err != nil {
		t.Errorf("SetTicker: expected nil, got %v", err)
	}
	if sw.IsLive() {
		t.Error("IsLive: expected false for a passthrough writer")
	}
	if sw.GetWidth() == nil {
		t.Fatal("GetWidth: expected a non-nil func even for a passthrough writer")
	}
	if got := sw.GetWidth()(); got != -1 {
		t.Errorf("GetWidth: expected a func reporting -1 for a passthrough writer, got %d", got)
	}
	if err := sw.Close(); err != nil {
		t.Errorf("Close: expected nil, got %v", err)
	}
	select {
	case _, ok := <-sw.Err():
		if ok {
			t.Error("Err: expected an already-closed channel for a passthrough writer")
		}
	default:
		t.Error("Err: expected an already-closed channel for a passthrough writer, got one that would block")
	}
}

func TestWriterPassthrough_WritePassesThroughUnmodified(t *testing.T) {
	buf := &syncBuffer{}
	sw := WriterPassthrough{Writer: buf}

	n, err := sw.Write([]byte("hello\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 6 {
		t.Errorf("expected 6 bytes written, got %d", n)
	}
	if buf.String() != "hello\n" {
		t.Errorf("expected the underlying writer to receive the data verbatim, got %q", buf.String())
	}
}

func TestWriterReal_IsLive(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	if !asReal(t, pair.Standard).IsLive() {
		t.Error("IsLive: expected true for a writer backed by a live spinner actor")
	}
	if !pair.Spinner.IsLive() {
		t.Error("IsLive: expected true for a writer backed by a live spinner actor")
	}
}

func TestWriterReal_GetWidth_DefaultsToNegativeOneWithoutResizeDetection(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	if pair.Spinner.GetWidth() == nil {
		t.Fatal("expected a non-nil func even when resize detection isn't configured")
	}
	if got := pair.Spinner.GetWidth()(); got != -1 {
		t.Errorf("expected GetWidth()() to report -1 when resize detection isn't configured, got %d", got)
	}
}

func TestWriterReal_GetWidth_ExposesTheConfiguredGetWidthOnBothWriters(t *testing.T) {
	getWidth := func() int { return 55 }

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time), WrapWithResizeDetection(getWidth))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	if got := pair.Spinner.GetWidth()(); got != 55 {
		t.Errorf("expected Spinner.GetWidth() to report the configured getWidth, got %d", got)
	}
	if got := asReal(t, pair.Standard).GetWidth()(); got != 55 {
		t.Errorf("expected Standard's underlying writer to also carry the configured getWidth, got %d", got)
	}
}

func TestStart_Idempotent(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	ctx := context.Background()
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(ctx) })
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(ctx) })
	if err != nil {
		t.Fatalf("second start should be a no-op, got error: %v", err)
	}
}

func TestStart_RedundantCallsDoNotLeakWatcherGoroutines(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	before := runtime.NumGoroutine()

	const redundantCalls = 20
	for i := range redundantCalls {
		callWithTimeout(t, 2*time.Second, "Start", func() {
			if err := pair.Spinner.Start(context.Background()); err != nil {
				t.Fatalf("redundant start #%d: %v", i, err)
			}
		})
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	after := runtime.NumGoroutine()

	if after > before {
		t.Errorf("expected %d redundant Start() calls to spawn no new goroutines, but goroutine count grew from %d to %d (+%d) — goroutine dump:\n%s", redundantCalls, before, after, after-before, dumpGoroutines())
	}
}

func TestStart_StopCycleDoesNotLeakWatcherGoroutines(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	before := runtime.NumGoroutine()

	const cycles = 20
	for i := range cycles {
		callWithTimeout(t, 2*time.Second, "Start", func() {
			if err := pair.Spinner.Start(context.Background()); err != nil {
				t.Fatalf("start #%d: %v", i, err)
			}
		})
		callWithTimeout(t, 2*time.Second, "Stop", func() {
			if err := pair.Spinner.Stop(); err != nil {
				t.Fatalf("stop #%d: %v", i, err)
			}
		})
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	after := runtime.NumGoroutine()

	if after > before {
		t.Errorf("expected %d Start()/Stop() cycles to spawn no lingering goroutines, but goroutine count grew from %d to %d (+%d) — goroutine dump:\n%s", cycles, before, after, after-before, dumpGoroutines())
	}
}

func TestStart_ToleratesFrameFuncError(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, errorFrame(errors.New("boom")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("expected Start to tolerate a FrameFunc error, got %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })
}

func TestStop_AfterStartWithAlwaysFailingFrameFuncDoesNotHang(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, errorFrame(errors.New("boom")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })
}

func TestStart_DrawsInitialFrameImmediately(t *testing.T) {
	spinner := &syncBuffer{}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("*spin*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	if got := spinner.String(); got != "*spin*" {
		t.Errorf("expected initial frame to be drawn immediately, got %q", got)
	}
}

func TestStart_DrawsInitialFrameEvenIfEqualToFrameFromPriorRun(t *testing.T) {
	spinner := &syncBuffer{}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("X")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer func() { _ = pair.Spinner.Close() }()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start 1: %v", err)
	}
	waitForCondition(t, func() bool { return spinner.Len() > 0 })

	callWithTimeout(t, 2*time.Second, "Stop", func() { err = pair.Spinner.Stop() })
	if err != nil {
		t.Fatalf("stop: %v", err)
	}

	spinner.Reset()
	callWithTimeout(t, 2*time.Second, "Start2", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start 2: %v", err)
	}

	if got := spinner.String(); got != "X" {
		t.Errorf("expected the restarted spinner to draw its first frame even though it matches the frame from before Stop, got %q", got)
	}
}

func TestStop_DoesNotLeaveNeedClearStaleForLaterWrite(t *testing.T) {
	shared := &syncBuffer{}

	pair, err := WrapPair(context.Background(), shared, shared, staticFrame([]byte("X")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer func() { _ = pair.Spinner.Close() }()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForCondition(t, func() bool { return shared.Len() > 0 })

	callWithTimeout(t, 2*time.Second, "Stop", func() { err = pair.Spinner.Stop() })
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	afterStop := shared.String()

	if _, err := pair.Standard.Write([]byte("hi\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if want := afterStop + "hi\n"; shared.String() != want {
		t.Errorf("write after Stop redundantly re-cleared an already-blank line, got %q, want %q", shared.String(), want)
	}
}

func TestStop_WithoutStart(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	var stopErr error
	callWithTimeout(t, 2*time.Second, "Stop", func() { stopErr = pair.Spinner.Stop() })
	if stopErr != nil {
		t.Fatalf("expected nil error stopping an unstarted spinner, got %v", stopErr)
	}
}

func TestStop_ClearsSpinner(t *testing.T) {
	spinner := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	ticker <- time.Now()
	waitForCondition(t, func() bool { return spinner.Len() > 0 })

	var stopErr error
	callWithTimeout(t, 2*time.Second, "Stop", func() { stopErr = pair.Spinner.Stop() })
	if stopErr != nil {
		t.Fatalf("stop: %v", stopErr)
	}

	if got := spinner.String(); !strings.HasSuffix(got, string(clearLineBytes)) {
		t.Errorf("expected output to end with the clear sequence, got %q", got)
	}
}

func TestStop_WithResizeAwareDrawer_ClearsAllWrappedRowsNotJustOne(t *testing.T) {
	var width atomic.Int64
	width.Store(40)
	getWidth := func() int { return int(width.Load()) }

	spinner := &syncBuffer{}
	wideFrame := []byte(strings.Repeat("X", 40))

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame(wideFrame), make(chan time.Time), WrapWithResizeDetection(getWidth))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	width.Store(10)

	var stopErr error
	callWithTimeout(t, 2*time.Second, "Stop", func() { stopErr = pair.Spinner.Stop() })
	if stopErr != nil {
		t.Fatalf("stop: %v", stopErr)
	}

	want := string(clearLineBytes) + strings.Repeat(string(clearPrevLine), 3)
	if got := spinner.String(); !strings.HasSuffix(got, want) {
		t.Errorf("expected Stop to clear all 4 wrapped rows (%q), got output ending in %q", want, got)
	}
}

func TestStopNoClear_CtxCancelDuringFinalFetch_DoesNotWedgeActor(t *testing.T) {
	writeErr := errors.New("terminal gone")
	spinner := &failAfterWriter{n: 1, err: writeErr}

	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	frame := func() ([]byte, error) {
		if calls.Add(1) == 1 {
			return []byte("AAAA"), nil
		}
		close(entered)
		<-release
		return []byte("BBBB"), nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, frame, make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	real := asReal(t, pair.Spinner)

	stopReturned := make(chan struct{})
	go func() {
		defer close(stopReturned)
		_ = pair.Spinner.StopNoClear("")
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("final frame fetch never started")
	}

	real.st.close()

	select {
	case <-stopReturned:
	case <-time.After(2 * time.Second):
		t.Fatalf("StopNoClear did not return after ctx cancel - goroutine dump:\n%s", dumpGoroutines())
	}

	close(release)

	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })
}

func TestStop_ThenWrite_DoesNotResurrectStaleFrame(t *testing.T) {
	shared := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), shared, shared, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForCondition(t, func() bool { return strings.Contains(shared.String(), "*") })

	callWithTimeout(t, 2*time.Second, "Stop", func() { err = pair.Spinner.Stop() })
	if err != nil {
		t.Fatalf("stop: %v", err)
	}

	afterStop := shared.String()
	if strings.HasSuffix(afterStop, "*") {
		t.Fatalf("frame character still visible right after Stop: %q", afterStop)
	}

	if _, err := pair.Standard.Write([]byte("goodbye\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := shared.String(); strings.HasSuffix(got, "*") {
		t.Errorf("stale spinner frame resurrected on screen by a post-Stop write: %q", got)
	}
}

func TestStopWith(t *testing.T) {
	spinner := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopWith", func() { stopErr = pair.Spinner.StopWith("done") })
	if stopErr != nil {
		t.Fatalf("stopWith: %v", stopErr)
	}

	if got := spinner.String(); !strings.Contains(got, "done") {
		t.Errorf("expected final frame %q in output, got %q", "done", got)
	}
}

func TestStopNoClear_AdoptsFreshFrameOnSuccess(t *testing.T) {
	spinner := &syncBuffer{}
	var frame atomic.Pointer[[]byte]
	first := []byte("1%")
	frame.Store(&first)
	frameFn := func() ([]byte, error) { return *frame.Load(), nil }

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, frameFn, make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	second := []byte("99%")
	frame.Store(&second)

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinner.StopNoClear("") })
	if stopErr != nil {
		t.Fatalf("stopNoClear: %v", stopErr)
	}

	if got := spinner.String(); !strings.HasSuffix(got, "99%") {
		t.Errorf("expected StopNoClear to adopt the freshest frame before freezing, got %q", got)
	}
}

func TestStopNoClear_RedrawsFreshFrameInPlaceOverOldOne(t *testing.T) {
	spinner := &syncBuffer{}
	var frame atomic.Pointer[[]byte]
	first := []byte("1%")
	frame.Store(&first)
	frameFn := func() ([]byte, error) { return *frame.Load(), nil }

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, frameFn, make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer func() { _ = pair.Spinner.Close() }()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	before := spinner.String()

	second := []byte("99%")
	frame.Store(&second)

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinner.StopNoClear("") })
	if stopErr != nil {
		t.Fatalf("stopNoClear: %v", stopErr)
	}

	if want := before + string(clearLineBytes) + "99%"; spinner.String() != want {
		t.Errorf("expected the fresh frame to clear and replace the old one in place, got %q, want %q", spinner.String(), want)
	}
}

func TestStopNoClear_FrozenFrameIsNotRedrawnByLaterWrites(t *testing.T) {
	shared := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), shared, shared, staticFrame([]byte("99%")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinner.StopNoClear(" done\n") })
	if stopErr != nil {
		t.Fatalf("stopNoClear: %v", stopErr)
	}

	if _, err := pair.Standard.Write([]byte("some later output\n")); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	if _, err := pair.Standard.Write([]byte("even later output\n")); err != nil {
		t.Fatalf("write 2: %v", err)
	}

	if got := shared.String(); strings.Count(got, "99%") > 1 {
		t.Errorf("frozen final frame was redrawn by later, unrelated writes: %q", got)
	}
}

func TestStopNoClear_PreservesLastFrameOnFetchFailure(t *testing.T) {
	spinner := &syncBuffer{}
	var fail atomic.Bool
	frameFn := func() ([]byte, error) {
		if fail.Load() {
			return nil, errors.New("boom")
		}
		return []byte("42%"), nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, frameFn, make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	before := spinner.String()
	fail.Store(true)

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinner.StopNoClear("") })
	after := spinner.String()

	if stopErr != nil {
		t.Fatalf("stopNoClear: %v", stopErr)
	}
	if before != after {
		t.Errorf("expected StopNoClear to leave the last known good frame untouched when the final fetch fails, before=%q after=%q", before, after)
	}
}

func TestStopNoClear_FreshFrameWriteFailurePropagates(t *testing.T) {
	writeErr := errors.New("stop redraw boom")
	spinner := &failAfterWriter{n: 1, err: writeErr}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, Simple([]string{"a", "b"}), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinner.StopNoClear("") })
	if !errors.Is(stopErr, writeErr) {
		t.Errorf("expected StopNoClear to propagate the write failure, got %v", stopErr)
	}
}

func TestStopNoClear_FreshFrameSetFailure_DoesNotResurrectStaleFrame(t *testing.T) {
	writeErr := errors.New("stop redraw boom")
	shared := &failOnCallWriter{on: 2, err: writeErr}

	pair, err := WrapPair(context.Background(), shared, shared, Simple([]string{"a", "b"}), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinner.StopNoClear("") })
	if !errors.Is(stopErr, writeErr) {
		t.Fatalf("expected StopNoClear to propagate the write failure, got %v", stopErr)
	}

	if _, err := pair.Standard.Write([]byte("later output\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := shared.String(); strings.HasSuffix(got, "a") {
		t.Errorf("stale pre-stop frame resurrected by a later write after StopNoClear's own redraw failed: %q", got)
	}
}

func TestStopNoClear_JoinsSlowInFlightFetchInsteadOfFetchingFresh(t *testing.T) {
	shared := &syncBuffer{}
	ticker := make(chan time.Time)
	release := make(chan struct{})
	var callCount atomic.Int32
	var value atomic.Pointer[string]
	initial := "50%"
	value.Store(&initial)

	frameFn := func() ([]byte, error) {
		callCount.Add(1)
		v := *value.Load()
		<-release
		return []byte(v), nil
	}

	pair, err := WrapPair(context.Background(), shared, shared, frameFn, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	go func() { release <- struct{}{} }()
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForCondition(t, func() bool { return strings.Contains(shared.String(), "50%") })

	ticker <- time.Now()
	waitForCondition(t, func() bool { return callCount.Load() == 2 })

	newVal := "100%"
	value.Store(&newVal)

	stopDone := make(chan error, 1)
	go func() { stopDone <- pair.Spinner.StopNoClear("") }()

	time.Sleep(100 * time.Millisecond)
	close(release)

	var stopErr error
	select {
	case stopErr = <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("StopNoClear did not return")
	}
	if stopErr != nil {
		t.Fatalf("stopNoClear: %v", stopErr)
	}

	if got := shared.String(); !strings.HasSuffix(got, "100%") {
		t.Errorf("StopNoClear froze on a stale in-flight value instead of fetching truly fresh (callCount=%d): %q", callCount.Load(), got)
	}
}

func TestStop_ClearWriteFailurePropagates(t *testing.T) {
	writeErr := errors.New("stop clear boom")
	spinner := &failAfterWriter{n: 1, err: writeErr}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var stopErr error
	callWithTimeout(t, 2*time.Second, "Stop", func() { stopErr = pair.Spinner.Stop() })
	if !errors.Is(stopErr, writeErr) {
		t.Errorf("expected Stop to propagate the clear write failure, got %v", stopErr)
	}
}

func TestStopWith_JoinsClearAndMessageWriteFailures(t *testing.T) {
	writeErr := errors.New("stop boom")
	spinner := &failAfterWriter{n: 1, err: writeErr}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopWith", func() { stopErr = pair.Spinner.StopWith("bye") })
	if !errors.Is(stopErr, writeErr) {
		t.Fatalf("expected StopWith to propagate the failure, got %v", stopErr)
	}
	if got := strings.Count(stopErr.Error(), writeErr.Error()); got != 2 {
		t.Errorf("expected both the clear() and message write failures joined into one error (message appearing twice), got %q (count=%d)", stopErr.Error(), got)
	}
}

func TestStopNoClear_WritesSuffixWithoutClearing(t *testing.T) {
	spinner := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	before := spinner.String()
	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinner.StopNoClear("bye") })
	after := spinner.String()

	if stopErr != nil {
		t.Fatalf("stopNoClear: %v", stopErr)
	}
	if want := before + "bye"; after != want {
		t.Errorf("expected the frozen frame followed by the raw suffix with no clear sequence in between, got %q, want %q", after, want)
	}
}

func TestStopNoClear_CommitsUnchangedFrameAfterPartialWrite(t *testing.T) {
	shared := &syncBuffer{}

	pair, err := WrapPair(context.Background(), shared, shared, staticFrame([]byte("hehe")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if _, err := pair.Standard.Write([]byte("abc")); err != nil {
		t.Fatalf("write abc: %v", err)
	}
	before := shared.String()

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinner.StopNoClear("hoho") })
	if stopErr != nil {
		t.Fatalf("stopNoClear: %v", stopErr)
	}

	after := strings.TrimPrefix(shared.String(), before)
	if !strings.Contains(after, "hehe") {
		t.Errorf("expected the outgoing frame to be committed to scrollback again at Stop time (nothing on screen currently backs the stale st.frame after the partial write), but nothing new after %q was written except %q", before, after)
	}
}

func TestSetFrame_RedrawsFrameDifferingOnlyInCase(t *testing.T) {
	spinner := &syncBuffer{}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("ABC")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	callWithTimeout(t, 2*time.Second, "SetFrame", func() { err = pair.Spinner.SetFrame(staticFrame([]byte("abc"))) })
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	if got := spinner.String(); !strings.Contains(got, "abc") {
		t.Errorf("expected the new frame %q to be drawn, got %q", "abc", got)
	}
}

func TestSetFrame_UpdatesDisplayedFrame(t *testing.T) {
	spinner := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("a")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	callWithTimeout(t, 2*time.Second, "SetFrame", func() { err = pair.Spinner.SetFrame(staticFrame([]byte("b"))) })
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	if got := spinner.String(); !strings.Contains(got, "b") {
		t.Errorf("expected updated frame %q in output, got %q", "b", got)
	}
}

func TestSetFrame_PropagatesWriteError(t *testing.T) {
	wantErr := errors.New("disk full")
	spinner := &failAfterWriter{n: 1, err: wantErr}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("a")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	callWithTimeout(t, 2*time.Second, "SetFrame", func() { err = pair.Spinner.SetFrame(staticFrame([]byte("b"))) })
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected SetFrame to propagate the write error, got %v", err)
	}
}

func TestSetFrame_NilFrameFuncErrors(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("a")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "SetFrame", func() { err = pair.Spinner.SetFrame(nil) })
	if err == nil {
		t.Error("expected error when setting a nil frame func")
	}
}

func TestSetFrameWith_WritesMessageThenDrawsNewFrame(t *testing.T) {
	spinner := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("a")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer func() { _ = pair.Spinner.Close() }()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	before := spinner.String()

	callWithTimeout(t, 2*time.Second, "SetFrameWith", func() {
		err = pair.Spinner.SetFrameWith(staticFrame([]byte("b")), "msg\n")
	})
	if err != nil {
		t.Fatalf("setFrameWith: %v", err)
	}

	if want := before + string(clearLineBytes) + "msg\n" + "b"; spinner.String() != want {
		t.Errorf("expected clear, then the message, then the new frame drawn, got %q, want %q", spinner.String(), want)
	}
}

func TestSetFrameWith_NilFrameFuncErrors(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("a")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "SetFrameWith", func() { err = pair.Spinner.SetFrameWith(nil, "msg") })
	if err == nil {
		t.Error("expected error when setting a nil frame func")
	}
}

func TestSetFrameWith_PropagatesStillFrameWriteError(t *testing.T) {
	wantErr := errors.New("disk full")
	spinner := &failOnCallWriter{on: 3, err: wantErr}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("a")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "SetFrameWith", func() {
		err = pair.Spinner.SetFrameWith(staticFrame([]byte("b")), "msg\n")
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected SetFrameWith to propagate the message write failure, got %v", err)
	}
}

func TestSetFrameWith_PropagatesNewFrameDrawError(t *testing.T) {
	wantErr := errors.New("disk full")
	spinner := &failOnCallWriter{on: 4, err: wantErr}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("a")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "SetFrameWith", func() {
		err = pair.Spinner.SetFrameWith(staticFrame([]byte("b")), "msg\n")
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected SetFrameWith to propagate the new frame's draw failure, got %v", err)
	}
	if !strings.Contains(spinner.String(), "msg\n") {
		t.Errorf("expected the message to have been committed before the failing draw, got %q", spinner.String())
	}
}

func TestSetFrameWith_PropagatesWriteError(t *testing.T) {
	wantErr := errors.New("disk full")
	spinner := &failAfterWriter{n: 1, err: wantErr}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("a")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	callWithTimeout(t, 2*time.Second, "SetFrameWith", func() {
		err = pair.Spinner.SetFrameWith(staticFrame([]byte("b")), "msg\n")
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected SetFrameWith to propagate the write failure, got %v", err)
	}
}

func TestSetFrameNoClear_RedrawsChangedFrameBeforeSuffix(t *testing.T) {
	spinner := &syncBuffer{}
	ticker := make(chan time.Time)

	var cur atomic.Pointer[[]byte]
	first := []byte("old%")
	cur.Store(&first)
	frameFn := func() ([]byte, error) { return *cur.Load(), nil }

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, frameFn, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer func() { _ = pair.Spinner.Close() }()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	before := spinner.String()

	fresh := []byte("fresh%")
	cur.Store(&fresh)

	callWithTimeout(t, 2*time.Second, "SetFrameNoClear", func() {
		err = pair.Spinner.SetFrameNoClear(staticFrame([]byte("next%")), " done")
	})
	if err != nil {
		t.Fatalf("setFrameNoClear: %v", err)
	}

	if want := before + string(clearLineBytes) + "fresh%" + " done" + "\n" + "next%"; spinner.String() != want {
		t.Errorf("expected the changed outgoing frame to be redrawn fresh before the suffix, a trailing newline forced onto the suffix (since it didn't already end in one), then the new frame, got %q, want %q", spinner.String(), want)
	}
}

func TestSetFrameNoClear_WritesSuffixWithoutClearingWhenFrameUnchanged(t *testing.T) {
	spinner := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("same%")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer func() { _ = pair.Spinner.Close() }()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	before := spinner.String()

	callWithTimeout(t, 2*time.Second, "SetFrameNoClear", func() {
		err = pair.Spinner.SetFrameNoClear(staticFrame([]byte("next%")), " done")
	})
	if err != nil {
		t.Fatalf("setFrameNoClear: %v", err)
	}

	if want := before + " done" + "\n" + "next%"; spinner.String() != want {
		t.Errorf("expected the suffix appended with no clear sequence, a trailing newline forced onto it, then the new frame with no clear before it either, got %q, want %q", spinner.String(), want)
	}
}

func TestSetFrameNoClear_NilFrameFuncErrors(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("a")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "SetFrameNoClear", func() { err = pair.Spinner.SetFrameNoClear(nil, "msg") })
	if err == nil {
		t.Error("expected error when setting a nil frame func")
	}
}

func TestSetFrameNoClear_InsertsSafetyNewlineAfterPartialWrite(t *testing.T) {
	shared := &syncBuffer{}
	ticker := make(chan time.Time)

	var cur atomic.Pointer[[]byte]
	first := []byte("hehe")
	cur.Store(&first)
	frameFn := func() ([]byte, error) { return *cur.Load(), nil }

	pair, err := WrapPair(context.Background(), shared, shared, frameFn, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer func() { _ = pair.Spinner.Close() }()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if _, err := pair.Standard.Write([]byte("abc")); err != nil {
		t.Fatalf("write abc: %v", err)
	}
	before := shared.String()

	changed := []byte("fresh")
	cur.Store(&changed)

	callWithTimeout(t, 2*time.Second, "SetFrameNoClear", func() {
		err = pair.Spinner.SetFrameNoClear(staticFrame([]byte("haha")), "hoho")
	})
	if err != nil {
		t.Fatalf("setFrameNoClear: %v", err)
	}

	if want := before + "\n" + "fresh" + "hoho" + "\n" + "haha"; shared.String() != want {
		t.Errorf("expected a safety newline before the committed frame+suffix (since the previous write hadn't ended in one), a trailing newline forced onto the suffix, then the new frame drawn immediately, got %q, want %q", shared.String(), want)
	}
}

func TestSetFrameWith_InsertsSafetyNewlineAfterPartialWrite(t *testing.T) {
	shared := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), shared, shared, staticFrame([]byte("hehe")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer func() { _ = pair.Spinner.Close() }()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if _, err := pair.Standard.Write([]byte("abc")); err != nil {
		t.Fatalf("write abc: %v", err)
	}
	before := shared.String()

	callWithTimeout(t, 2*time.Second, "SetFrameWith", func() {
		err = pair.Spinner.SetFrameWith(staticFrame([]byte("haha")), "msg\n")
	})
	if err != nil {
		t.Fatalf("setFrameWith: %v", err)
	}

	if want := before + "\n" + "msg\n" + "haha"; shared.String() != want {
		t.Errorf("expected a safety newline before the committed message (since the previous write hadn't ended in one) and the new frame drawn immediately after (the message already ended in \\n, so no extra trailing newline is forced), got %q, want %q", shared.String(), want)
	}
}

func TestSetFrameNoClear_ResumesAnimatingAfterUnchangedFrame(t *testing.T) {
	spinner := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("same%")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer func() { _ = pair.Spinner.Close() }()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "SetFrameNoClear", func() {
		err = pair.Spinner.SetFrameNoClear(staticFrame([]byte("next%")), " done")
	})
	if err != nil {
		t.Fatalf("setFrameNoClear: %v", err)
	}

	ticker <- time.Now()
	waitForCondition(t, func() bool { return strings.Contains(spinner.String(), "next%") })
}

func TestWrite_ClearsAndRedrawsSpinner(t *testing.T) {
	main := &syncBuffer{}
	spinner := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), main, spinner, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	callWithTimeout(t, 2*time.Second, "SetFrame", func() { err = pair.Spinner.SetFrame(staticFrame([]byte("*"))) })
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if spinner.Len() == 0 {
		t.Fatal("expected a frame to be drawn before write")
	}

	line := []byte("log line\n")
	n, err := pair.Standard.Write(line)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != len(line) {
		t.Errorf("expected %d bytes written, got %d", len(line), n)
	}

	if got := main.String(); got != string(line) {
		t.Errorf("expected data on main writer, got %q", got)
	}

	got := spinner.String()
	if !strings.Contains(got, string(clearLineBytes)) {
		t.Errorf("expected clear sequence on spinner stream, got %q", got)
	}
	if !strings.HasSuffix(got, "*") {
		t.Errorf("expected spinner to be redrawn after a newline-terminated write, got %q", got)
	}
}

func TestWrite_NoRedrawWithoutTrailingNewline(t *testing.T) {
	main := &syncBuffer{}
	spinner := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), main, spinner, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	callWithTimeout(t, 2*time.Second, "SetFrame", func() { err = pair.Spinner.SetFrame(staticFrame([]byte("*"))) })
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	spinner.Reset()

	if _, err := pair.Standard.Write([]byte("partial")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := spinner.String(); got != string(clearLineBytes) {
		t.Errorf("expected only the clear sequence (no redraw) after a write with no trailing newline, got %q", got)
	}
}

func TestWrite_PropagatesUnderlyingError(t *testing.T) {
	wantErr := errors.New("disk full")

	pair, err := WrapPair(context.Background(), errWriter{wantErr}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	if _, err := pair.Standard.Write([]byte("hi")); !errors.Is(err, wantErr) {
		t.Errorf("expected %v, got %v", wantErr, err)
	}
}

func TestWrite_ClearFailureDoesNotAbortPayloadWrite(t *testing.T) {
	writeErr := errors.New("write-clear boom")
	spinner := &failAfterWriter{n: 1, err: writeErr}
	main := &syncBuffer{}

	pair, err := WrapPair(context.Background(), main, spinner, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	n, err := pair.Standard.Write([]byte("hi\n"))
	if err != nil {
		t.Fatalf("expected the payload write to succeed despite the clear failure, got %v", err)
	}
	if n != len("hi\n") {
		t.Errorf("expected 3 bytes written, got %d", n)
	}
	if got := main.String(); got != "hi\n" {
		t.Errorf("expected payload on main writer, got %q", got)
	}

	select {
	case gotErr := <-pair.Spinner.Err():
		if !errors.Is(gotErr, writeErr) {
			t.Errorf("expected the clear failure to surface on Err(), got %v", gotErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected the clear failure to be reported on Err()")
	}
}

func TestWriteError_AutoStopDoesNotResurrectStaleFrameOnLaterWrite(t *testing.T) {
	writeErr := errors.New("boom")
	w := &failOnCallWriter{on: 2, err: writeErr}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), w, w, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer func() { _ = pair.Spinner.Close() }()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForCondition(t, func() bool { return strings.Contains(w.String(), "*") })

	if _, err := pair.Spinner.Write([]byte("partial")); err != nil {
		t.Fatalf("expected payload write to succeed despite clear failure, got %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	if _, err := pair.Spinner.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := w.String(); strings.Count(got, "*") > 1 {
		t.Errorf("stale frame resurrected by a write after write-error auto-stop: %q", got)
	}
}

func TestWriteError_AutoStopClosesNotifyStoppedWatcher(t *testing.T) {
	const marker = "spinnerState).start.func1"
	countMarker := func() int { return strings.Count(dumpGoroutines(), marker) }

	var baseline int
	waitForCondition(t, func() bool {
		a := countMarker()
		time.Sleep(20 * time.Millisecond)
		b := countMarker()
		baseline = b
		return a == b
	})

	writeErr := errors.New("boom")
	w := &failOnCallWriter{on: 2, err: writeErr}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), w, w, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer func() { _ = pair.Spinner.Close() }()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForCondition(t, func() bool { return strings.Contains(w.String(), "*") })
	waitForCondition(t, func() bool { return countMarker() == baseline+1 })

	_, _ = pair.Spinner.Write([]byte("hello\n"))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && countMarker() != baseline {
		time.Sleep(20 * time.Millisecond)
	}
	if got := countMarker(); got != baseline {
		t.Errorf("ctx-watcher goroutine from Start still alive after write-error auto-stop (marker count=%d, baseline=%d): notifyStopped was not closed", got, baseline)
	}
}

func TestWrite_DrawFailureReportsTheDrawError(t *testing.T) {
	drawErr := errors.New("draw boom")
	w := &failOnCallWriter{on: 4, err: drawErr}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), w, w, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer func() { _ = pair.Spinner.Close() }()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForCondition(t, func() bool { return strings.Contains(w.String(), "*") })

	if _, err := pair.Spinner.Write([]byte("hi\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case gotErr := <-pair.Spinner.Err():
		if !errors.Is(gotErr, drawErr) {
			t.Errorf("expected the reported error to wrap the draw failure, got %v", gotErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected the draw failure to be reported on Err()")
	}
}

func TestTickerUpdatesFrame(t *testing.T) {
	spinner := &syncBuffer{}
	ticker := make(chan time.Time)
	frames := [][]byte{[]byte("a"), []byte("b"), []byte("c")}
	idx := 0
	frameFn := func() ([]byte, error) {
		f := frames[idx%len(frames)]
		idx++
		return f, nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, frameFn, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	ticker <- time.Now()
	waitForCondition(t, func() bool { return strings.Contains(spinner.String(), "b") })

	ticker <- time.Now()
	waitForCondition(t, func() bool { return strings.Contains(spinner.String(), "c") })
}

func TestTicker_FrameErrorSkipsUpdate(t *testing.T) {
	spinner := &syncBuffer{}
	ticker := make(chan time.Time)
	var fail atomic.Bool
	frameFn := func() ([]byte, error) {
		if fail.Load() {
			return nil, errors.New("temporary failure")
		}
		return []byte("ok"), nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, frameFn, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	fail.Store(true)
	ticker <- time.Now()
	time.Sleep(20 * time.Millisecond)

	fail.Store(false)
	ticker <- time.Now()
	waitForCondition(t, func() bool { return strings.Contains(spinner.String(), "ok") })
}

func TestTicker_IgnoredWhileNotRunning(t *testing.T) {
	var calls atomic.Int32
	frameFn := func() ([]byte, error) {
		calls.Add(1)
		return []byte("*"), nil
	}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, frameFn, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	ticker <- time.Now()
	time.Sleep(50 * time.Millisecond)

	if got := calls.Load(); got != 0 {
		t.Errorf("expected getFrame not to be called for a tick while not running, got %d call(s)", got)
	}
}

func TestWrapPair_NilTicker_NoPeriodicRedraw(t *testing.T) {
	var calls atomic.Int64
	frameFn := func() ([]byte, error) {
		return []byte(fmt.Sprintf("f%d", calls.Add(1))), nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, frameFn, nil)
	if err != nil {
		t.Fatalf("WrapPair with a nil ticker: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected exactly one frame fetch on Start, got %d", got)
	}

	time.Sleep(60 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Errorf("a nil ticker must not drive periodic redraws, but getFrame was called %d times", got)
	}

	callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })
	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })
}

func TestTicker_ClosedChannelStopsDrivingRedraws(t *testing.T) {
	var calls atomic.Int64
	frameFn := func() ([]byte, error) {
		return []byte(fmt.Sprintf("f%d", calls.Add(1))), nil
	}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, frameFn, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	close(ticker)

	time.Sleep(40 * time.Millisecond)
	settled := calls.Load()
	time.Sleep(60 * time.Millisecond)
	if got := calls.Load(); got != settled {
		t.Errorf("a closed ticker must not keep driving redraws: getFrame calls went %d -> %d", settled, got)
	}

	callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })
	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })
}

func TestSetTicker_SwapsRedrawSource(t *testing.T) {
	spinner := &syncBuffer{}
	tickA := make(chan time.Time)
	tickB := make(chan time.Time)
	var calls atomic.Int64
	frameFn := func() ([]byte, error) {
		return []byte(fmt.Sprintf("f%d", calls.Add(1))), nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, frameFn, tickA)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	tickA <- time.Now()
	waitForCondition(t, func() bool { return strings.Contains(spinner.String(), "f2") })

	callWithTimeout(t, 2*time.Second, "SetTicker", func() { err = pair.Spinner.SetTicker(tickB) })
	if err != nil {
		t.Fatalf("SetTicker: %v", err)
	}

	tickB <- time.Now()
	waitForCondition(t, func() bool { return strings.Contains(spinner.String(), "f3") })

	select {
	case tickA <- time.Now():
		t.Fatal("the replaced ticker still drove a redraw after SetTicker")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSetTicker_NilFreezesThenResumesRedraws(t *testing.T) {
	spinner := &syncBuffer{}
	ticker := make(chan time.Time)
	var calls atomic.Int64
	frameFn := func() ([]byte, error) {
		return []byte(fmt.Sprintf("f%d", calls.Add(1))), nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, frameFn, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	ticker <- time.Now()
	waitForCondition(t, func() bool { return strings.Contains(spinner.String(), "f2") })

	callWithTimeout(t, 2*time.Second, "SetTicker(nil)", func() { err = pair.Spinner.SetTicker(nil) })
	if err != nil {
		t.Fatalf("SetTicker(nil): %v", err)
	}

	select {
	case ticker <- time.Now():
		t.Fatal("a redraw fired after SetTicker(nil) froze periodic redraws")
	case <-time.After(100 * time.Millisecond):
	}

	callWithTimeout(t, 2*time.Second, "SetTicker(live)", func() { err = pair.Spinner.SetTicker(ticker) })
	if err != nil {
		t.Fatalf("SetTicker(live): %v", err)
	}
	ticker <- time.Now()
	waitForCondition(t, func() bool { return strings.Contains(spinner.String(), "f3") })
}

func TestSetTicker_AfterCloseReturnsErrClosed(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	if err := pair.Spinner.SetTicker(make(chan time.Time)); !errors.Is(err, ErrClosed) {
		t.Errorf("expected ErrClosed from SetTicker after Close, got %v", err)
	}
}

func TestStart_NilContextFallsBackToSpinnersOwnContext(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(nil) }) //nolint:staticcheck
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })
}

func TestStart_CtxCancellationStopsSpinner(t *testing.T) {
	spinner := &syncBuffer{}
	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(ctx) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	cancel()

	waitForCondition(t, func() bool { return strings.HasSuffix(spinner.String(), string(clearLineBytes)) })
}

func TestFrameFunc_PanicHelperProcess(t *testing.T) {
	if os.Getenv("SPINQ_PANIC_HELPER") != "1" {
		t.Skip("only runs as a subprocess helper; see TestStart_PanickingFrameFuncDoesNotCrashProcess")
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, func() ([]byte, error) {
		panic("boom")
	}, make(chan time.Time))
	if err != nil {
		fmt.Fprintln(os.Stderr, "WrapPair:", err)
		os.Exit(1)
	}

	_ = pair.Spinner.Start(context.Background())
	fmt.Println("SURVIVED")
}

func TestStart_PanickingFrameFuncDoesNotCrashProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFrameFunc_PanicHelperProcess$", "-test.v")
	cmd.Env = append(os.Environ(), "SPINQ_PANIC_HELPER=1")
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("subprocess did not exit within the timeout (possible hang rather than a clean recover), output:\n%s", out)
	}
	if err != nil {
		t.Errorf("expected a panicking FrameFunc to be recovered instead of crashing the process: %v\noutput:\n%s", err, out)
		return
	}
	if !strings.Contains(string(out), "SURVIVED") {
		t.Errorf("expected the process to survive Start() and reach SURVIVED, output:\n%s", out)
	}
}

func TestStartNilContextHelperProcess(t *testing.T) {
	if os.Getenv("SPINQ_START_NIL_CTX_HELPER") != "1" {
		t.Skip("only runs as a subprocess helper; see TestStart_NilContextDoesNotCrashProcess")
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		fmt.Fprintln(os.Stderr, "WrapPair:", err)
		os.Exit(1)
	}

	_ = pair.Spinner.Start(nil) //nolint:staticcheck

	time.Sleep(200 * time.Millisecond)
	fmt.Println("SURVIVED")
}

func TestStart_NilContextDoesNotCrashProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStartNilContextHelperProcess$", "-test.v")
	cmd.Env = append(os.Environ(), "SPINQ_START_NIL_CTX_HELPER=1")
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("subprocess did not exit within the timeout, output:\n%s", out)
	}
	if err != nil {
		t.Errorf("expected Start(nil) to not crash the process: %v\noutput:\n%s", err, out)
		return
	}
	if !strings.Contains(string(out), "SURVIVED") {
		t.Errorf("expected the process to survive Start(nil) and reach SURVIVED, output:\n%s", out)
	}
}

func TestSetFrameWriteFailureThenCloseHelperProcess(t *testing.T) {
	if os.Getenv("SPINQ_DOUBLE_CLOSE_HELPER") != "1" {
		t.Skip("only runs as a subprocess helper; see TestSetFrame_WriteFailureThenCloseDoesNotCrashProcess")
	}

	spinner := &failAfterWriter{n: 1, err: errors.New("write boom")}
	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		fmt.Fprintln(os.Stderr, "WrapPair:", err)
		os.Exit(1)
	}

	if err := pair.Spinner.Start(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "Start:", err)
		os.Exit(1)
	}

	_ = pair.Spinner.SetFrame(staticFrame([]byte("different")))
	_ = pair.Spinner.Close()
	fmt.Println("SURVIVED")
}

func TestTickerPanicHelperProcess(t *testing.T) {
	if os.Getenv("SPINQ_TICKER_PANIC_HELPER") != "1" {
		t.Skip("only runs as a subprocess helper; see TestTicker_PanickingFrameFuncDoesNotCrashProcess")
	}

	var calls atomic.Int64
	frame := func() ([]byte, error) {
		if calls.Add(1) >= 2 {
			panic("boom: deliberate ticker-triggered FrameFunc panic")
		}
		return []byte("*"), nil
	}

	ticker := make(chan time.Time)
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, frame, ticker)
	if err != nil {
		fmt.Fprintln(os.Stderr, "WrapPair:", err)
		os.Exit(1)
	}

	if err := pair.Spinner.Start(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "Start:", err)
		os.Exit(1)
	}
	ticker <- time.Now()
	time.Sleep(200 * time.Millisecond)

	fmt.Println("SURVIVED")
}

func TestTicker_PanickingFrameFuncDoesNotCrashProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTickerPanicHelperProcess$", "-test.v")
	cmd.Env = append(os.Environ(), "SPINQ_TICKER_PANIC_HELPER=1")
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("subprocess did not exit within the timeout, output:\n%s", out)
	}
	if err != nil {
		t.Errorf("expected a panicking tick-triggered FrameFunc not to crash the process: %v\noutput:\n%s", err, out)
		return
	}
	if !strings.Contains(string(out), "SURVIVED") {
		t.Errorf("expected the process to survive a panicking tick-triggered FrameFunc, output:\n%s", out)
	}
}

func TestSetFramePanicHelperProcess(t *testing.T) {
	if os.Getenv("SPINQ_SETGETFRAME_PANIC_HELPER") != "1" {
		t.Skip("only runs as a subprocess helper; see TestSetFrame_PanickingFrameFuncDoesNotCrashProcess")
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		fmt.Fprintln(os.Stderr, "WrapPair:", err)
		os.Exit(1)
	}

	if err := pair.Spinner.Start(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "Start:", err)
		os.Exit(1)
	}

	panicky := func() ([]byte, error) { panic("boom: deliberate setGetFrame-triggered FrameFunc panic") }
	_ = pair.Spinner.SetFrame(panicky)

	fmt.Println("SURVIVED")
}

func TestSetFrame_PanickingFrameFuncDoesNotCrashProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSetFramePanicHelperProcess$", "-test.v")
	cmd.Env = append(os.Environ(), "SPINQ_SETGETFRAME_PANIC_HELPER=1")
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("subprocess did not exit within the timeout, output:\n%s", out)
	}
	if err != nil {
		t.Errorf("expected a panicking FrameFunc passed to SetFrame not to crash the process: %v\noutput:\n%s", err, out)
		return
	}
	if !strings.Contains(string(out), "SURVIVED") {
		t.Errorf("expected the process to survive a panicking FrameFunc passed to SetFrame, output:\n%s", out)
	}
}

func TestSetFrame_WriteFailureThenCloseDoesNotCrashProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSetFrameWriteFailureThenCloseHelperProcess$", "-test.v")
	cmd.Env = append(os.Environ(), "SPINQ_DOUBLE_CLOSE_HELPER=1")
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("subprocess did not exit within the timeout, output:\n%s", out)
	}
	if err != nil {
		t.Errorf("expected a write failure in SetFrame() followed by Close() not to crash the process: %v\noutput:\n%s", err, out)
		return
	}
	if !strings.Contains(string(out), "SURVIVED") {
		t.Errorf("expected the process to survive SetFrame() + Close(), output:\n%s", out)
	}
}

func TestClose_ClearsDisplay(t *testing.T) {
	spinner := &syncBuffer{}
	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	if got := spinner.String(); !strings.HasSuffix(got, string(clearLineBytes)) {
		t.Errorf("expected Close to clear the display, got %q", got)
	}
}

func TestClose_SubsequentCallsReturnErrClosed(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if !errors.Is(err, ErrClosed) {
		t.Errorf("expected Start after Close to return ErrClosed, got %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Stop", func() { err = pair.Spinner.Stop() })
	if !errors.Is(err, ErrClosed) {
		t.Errorf("expected Stop after Close to return ErrClosed, got %v", err)
	}

	callWithTimeout(t, 2*time.Second, "SetFrame", func() { err = pair.Spinner.SetFrame(staticFrame([]byte("x"))) })
	if !errors.Is(err, ErrClosed) {
		t.Errorf("expected SetFrame after Close to return ErrClosed, got %v", err)
	}
}

func TestClose_WaitsForInFlightTickFetchToReturn(t *testing.T) {
	spinner := &syncBuffer{}
	ticker := make(chan time.Time)
	proceed := make(chan struct{})
	var calls atomic.Int32
	frameFn := func() ([]byte, error) { //nolint:unparam
		if calls.Add(1) == 1 {
			return []byte("*"), nil
		}
		<-proceed
		return []byte("*"), nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, frameFn, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	ticker <- time.Now()
	waitForCondition(t, func() bool { return calls.Load() == 2 })

	closeReturned := make(chan struct{})
	go func() {
		_ = pair.Spinner.Close()
		close(closeReturned)
	}()

	select {
	case <-closeReturned:
		t.Fatal("expected Close to block while the in-flight tick fetch's FrameFunc call is still running, but it returned immediately")
	case <-time.After(100 * time.Millisecond):
	}

	close(proceed)

	select {
	case <-closeReturned:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected Close to return once the blocked FrameFunc call finally returned, but it never did — goroutine dump:\n%s", dumpGoroutines())
	}
}

func TestErr_ReceivesErrorOnDrawFrameWriteFailure(t *testing.T) {
	writeErr := errors.New("draw boom")
	spinner := &failAfterWriter{n: 1, err: writeErr}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, Simple([]string{"a", "b"}), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	ticker <- time.Now()

	select {
	case gotErr, ok := <-pair.Spinner.Err():
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
	spinner := &failAfterWriter{n: 1, err: writeErr}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	select {
	case gotErr, ok := <-pair.Spinner.Err():
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
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

	select {
	case _, ok := <-pair.Spinner.Err():
		if ok {
			t.Error("expected no buffered error since nothing failed, but got one")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected Err() to be immediately readable (closed) after Close, but it blocked")
	}
}

func TestErr_BestEffortSendDoesNotBlockCloseWhenBufferIsFull(t *testing.T) {
	writeErr := errors.New("always boom")
	spinner := &failAfterWriter{n: 1, err: writeErr}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, Simple([]string{"a", "b"}), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	ticker <- time.Now()
	time.Sleep(100 * time.Millisecond)

	start := time.Now()
	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })
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
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForCondition(t, func() bool { return strings.Contains(shared.String(), "*") })

	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })

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

func TestSetFrame_DoesNotCorruptPartialWrite(t *testing.T) {
	shared := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), shared, shared, staticFrame([]byte("a")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer func() { _ = pair.Spinner.Close() }()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if _, err := pair.Standard.Write([]byte("Uploading file.zip")); err != nil {
		t.Fatalf("partial write: %v", err)
	}
	before := shared.String()

	callWithTimeout(t, 2*time.Second, "SetFrame", func() { err = pair.Spinner.SetFrame(staticFrame([]byte("NEWFRAME"))) })
	if err != nil {
		t.Fatalf("setFrame: %v", err)
	}

	if got := shared.String(); got != before {
		t.Fatalf("expected SetFrame to draw nothing yet (still mid-line, no message to anchor a safety newline to), got %q appended after %q", got, before)
	}

	if _, err := pair.Standard.Write([]byte(" done\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if want := before + " done\nNEWFRAME"; shared.String() != want {
		t.Errorf("expected the deferred frame to draw once a real newline-terminated write completed the line, got %q, want %q", shared.String(), want)
	}
}

func TestSetFrameNoClear_CommitsUnchangedOutgoingFrameAfterPartialWrite(t *testing.T) {
	shared := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), shared, shared, staticFrame([]byte("hehe")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer func() { _ = pair.Spinner.Close() }()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if _, err := pair.Standard.Write([]byte("abc")); err != nil {
		t.Fatalf("write abc: %v", err)
	}
	before := shared.String()

	callWithTimeout(t, 2*time.Second, "SetFrameNoClear", func() {
		err = pair.Spinner.SetFrameNoClear(staticFrame([]byte("haha")), "hoho")
	})
	if err != nil {
		t.Fatalf("setFrameNoClear: %v", err)
	}

	if want := before + "\n" + "hehe" + "hoho" + "\n" + "haha"; shared.String() != want {
		t.Errorf("expected the unchanged outgoing frame to still be committed (on its own line, since nothing on screen backs it after the partial write), got %q, want %q", shared.String(), want)
	}
}
