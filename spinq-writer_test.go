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

func TestSpinqWriterPassthrough_AllLifecycleMethodsAreNoOps(t *testing.T) {
	sw := SpinqWriterPassthrough{Writer: &syncBuffer{}}

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
	if err := sw.Set(staticFrame([]byte("x"))); err != nil {
		t.Errorf("Set: expected nil, got %v", err)
	}
	if sw.IsReal() {
		t.Error("IsReal: expected false for a passthrough writer")
	}
	sw.close()
}

func TestSpinqWriterPassthrough_WritePassesThroughUnmodified(t *testing.T) {
	buf := &syncBuffer{}
	sw := SpinqWriterPassthrough{Writer: buf}

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

func TestSpinqWriterReal_IsReal(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

	if !pair.Standard.IsReal() {
		t.Error("IsReal: expected true for a writer backed by a live spinner actor")
	}
	if !pair.Spinny.IsReal() {
		t.Error("IsReal: expected true for a writer backed by a live spinner actor")
	}
}

func TestStart_Idempotent(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	ctx := context.Background()
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Standard.Start(ctx) })
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Standard.Stop() })

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Standard.Start(ctx) })
	if err != nil {
		t.Fatalf("second start should be a no-op, got error: %v", err)
	}
}

func TestStart_RedundantCallsDoNotLeakWatcherGoroutines(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	before := runtime.NumGoroutine()

	const redundantCalls = 20
	for i := range redundantCalls {
		callWithTimeout(t, 2*time.Second, "Start", func() {
			if err := pair.Spinny.Start(context.Background()); err != nil {
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
	defer callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	before := runtime.NumGoroutine()

	const cycles = 20
	for i := range cycles {
		callWithTimeout(t, 2*time.Second, "Start", func() {
			if err := pair.Spinny.Start(context.Background()); err != nil {
				t.Fatalf("start #%d: %v", i, err)
			}
		})
		callWithTimeout(t, 2*time.Second, "Stop", func() {
			if err := pair.Spinny.Stop(); err != nil {
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

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Standard.Start(context.Background()) })
	if err != nil {
		t.Fatalf("expected Start to tolerate a FrameFunc error, got %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Standard.Stop() })
}

func TestStop_AfterStartWithAlwaysFailingFrameFuncDoesNotHang(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, errorFrame(errors.New("boom")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Standard.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Standard.Stop() })
}

func TestStart_DrawsInitialFrameImmediately(t *testing.T) {
	spinny := &syncBuffer{}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, staticFrame([]byte("*spin*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinny.Stop() })

	if got := spinny.String(); got != "*spin*" {
		t.Errorf("expected initial frame to be drawn immediately, got %q", got)
	}
}

func TestStart_DrawsInitialFrameEvenIfEqualToFrameFromPriorRun(t *testing.T) {
	spinny := &syncBuffer{}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, staticFrame([]byte("X")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer pair.Close()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start 1: %v", err)
	}
	waitForCondition(t, func() bool { return spinny.Len() > 0 })

	callWithTimeout(t, 2*time.Second, "Stop", func() { err = pair.Spinny.Stop() })
	if err != nil {
		t.Fatalf("stop: %v", err)
	}

	spinny.Reset()
	callWithTimeout(t, 2*time.Second, "Start2", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start 2: %v", err)
	}

	if got := spinny.String(); got != "X" {
		t.Errorf("expected the restarted spinner to draw its first frame even though it matches the frame from before Stop, got %q", got)
	}
}

func TestStop_DoesNotLeaveNeedClearStaleForLaterWrite(t *testing.T) {
	shared := &syncBuffer{}

	pair, err := WrapPair(context.Background(), shared, shared, staticFrame([]byte("X")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer pair.Close()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForCondition(t, func() bool { return shared.Len() > 0 })

	callWithTimeout(t, 2*time.Second, "Stop", func() { err = pair.Spinny.Stop() })
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
	callWithTimeout(t, 2*time.Second, "Stop", func() { stopErr = pair.Standard.Stop() })
	if stopErr != nil {
		t.Fatalf("expected nil error stopping an unstarted spinner, got %v", stopErr)
	}
}

func TestStop_ClearsSpinner(t *testing.T) {
	spinny := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	ticker <- time.Now()
	waitForCondition(t, func() bool { return spinny.Len() > 0 })

	var stopErr error
	callWithTimeout(t, 2*time.Second, "Stop", func() { stopErr = pair.Spinny.Stop() })
	if stopErr != nil {
		t.Fatalf("stop: %v", stopErr)
	}

	if got := spinny.String(); !strings.HasSuffix(got, string(clearBytes)) {
		t.Errorf("expected output to end with the clear sequence, got %q", got)
	}
}

func TestStop_ThenWrite_DoesNotResurrectStaleFrame(t *testing.T) {
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

	callWithTimeout(t, 2*time.Second, "Stop", func() { err = pair.Spinny.Stop() })
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
	spinny := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopWith", func() { stopErr = pair.Spinny.StopWith("done") })
	if stopErr != nil {
		t.Fatalf("stopWith: %v", stopErr)
	}

	if got := spinny.String(); !strings.Contains(got, "done") {
		t.Errorf("expected final frame %q in output, got %q", "done", got)
	}
}

func TestStopNoClear_AdoptsFreshFrameOnSuccess(t *testing.T) {
	spinny := &syncBuffer{}
	var frame atomic.Pointer[[]byte]
	first := []byte("1%")
	frame.Store(&first)
	frameFn := func() ([]byte, error) { return *frame.Load(), nil }

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, frameFn, make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	second := []byte("99%")
	frame.Store(&second)

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinny.StopNoClear("") })
	if stopErr != nil {
		t.Fatalf("stopNoClear: %v", stopErr)
	}

	if got := spinny.String(); !strings.HasSuffix(got, "99%") {
		t.Errorf("expected StopNoClear to adopt the freshest frame before freezing, got %q", got)
	}
}

func TestStopNoClear_RedrawsFreshFrameInPlaceOverOldOne(t *testing.T) {
	spinny := &syncBuffer{}
	var frame atomic.Pointer[[]byte]
	first := []byte("1%")
	frame.Store(&first)
	frameFn := func() ([]byte, error) { return *frame.Load(), nil }

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, frameFn, make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer pair.Close()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	before := spinny.String()

	second := []byte("99%")
	frame.Store(&second)

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinny.StopNoClear("") })
	if stopErr != nil {
		t.Fatalf("stopNoClear: %v", stopErr)
	}

	if want := before + string(clearBytes) + "99%"; spinny.String() != want {
		t.Errorf("expected the fresh frame to clear and replace the old one in place, got %q, want %q", spinny.String(), want)
	}
}

func TestStopNoClear_FrozenFrameIsNotRedrawnByLaterWrites(t *testing.T) {
	shared := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), shared, shared, staticFrame([]byte("99%")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinny.StopNoClear(" done\n") })
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
	spinny := &syncBuffer{}
	var fail atomic.Bool
	frameFn := func() ([]byte, error) {
		if fail.Load() {
			return nil, errors.New("boom")
		}
		return []byte("42%"), nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, frameFn, make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	before := spinny.String()
	fail.Store(true)

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinny.StopNoClear("") })
	after := spinny.String()

	if stopErr != nil {
		t.Fatalf("stopNoClear: %v", stopErr)
	}
	if before != after {
		t.Errorf("expected StopNoClear to leave the last known good frame untouched when the final fetch fails, before=%q after=%q", before, after)
	}
}

func TestStopNoClear_FreshFrameWriteFailurePropagates(t *testing.T) {
	writeErr := errors.New("stop redraw boom")
	spinny := &failAfterWriter{n: 1, err: writeErr}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, Simple([]string{"a", "b"}), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinny.StopNoClear("") })
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
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinny.StopNoClear("") })
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
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForCondition(t, func() bool { return strings.Contains(shared.String(), "50%") })

	ticker <- time.Now()
	waitForCondition(t, func() bool { return callCount.Load() == 2 })

	newVal := "100%"
	value.Store(&newVal)

	stopDone := make(chan error, 1)
	go func() { stopDone <- pair.Spinny.StopNoClear("") }()

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
	spinny := &failAfterWriter{n: 1, err: writeErr}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var stopErr error
	callWithTimeout(t, 2*time.Second, "Stop", func() { stopErr = pair.Spinny.Stop() })
	if !errors.Is(stopErr, writeErr) {
		t.Errorf("expected Stop to propagate the clear write failure, got %v", stopErr)
	}
}

func TestStopNoClear_WritesSuffixWithoutClearing(t *testing.T) {
	spinny := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	before := spinny.String()
	var stopErr error
	callWithTimeout(t, 2*time.Second, "StopNoClear", func() { stopErr = pair.Spinny.StopNoClear("bye") })
	after := spinny.String()

	if stopErr != nil {
		t.Fatalf("stopNoClear: %v", stopErr)
	}
	if want := before + "bye"; after != want {
		t.Errorf("expected the frozen frame followed by the raw suffix with no clear sequence in between, got %q, want %q", after, want)
	}
}

func TestSet_RedrawsFrameDifferingOnlyInCase(t *testing.T) {
	spinny := &syncBuffer{}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, staticFrame([]byte("ABC")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinny.Stop() })

	callWithTimeout(t, 2*time.Second, "Set", func() { err = pair.Spinny.Set(staticFrame([]byte("abc"))) })
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	if got := spinny.String(); !strings.Contains(got, "abc") {
		t.Errorf("expected the new frame %q to be drawn, got %q", "abc", got)
	}
}

func TestSet_UpdatesDisplayedFrame(t *testing.T) {
	spinny := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, staticFrame([]byte("a")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinny.Stop() })

	callWithTimeout(t, 2*time.Second, "Set", func() { err = pair.Spinny.Set(staticFrame([]byte("b"))) })
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	if got := spinny.String(); !strings.Contains(got, "b") {
		t.Errorf("expected updated frame %q in output, got %q", "b", got)
	}
}

func TestSet_PropagatesWriteError(t *testing.T) {
	wantErr := errors.New("disk full")
	spinny := &failAfterWriter{n: 1, err: wantErr}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, staticFrame([]byte("a")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinny.Stop() })

	callWithTimeout(t, 2*time.Second, "Set", func() { err = pair.Spinny.Set(staticFrame([]byte("b"))) })
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected Set to propagate the write error, got %v", err)
	}
}

func TestSet_NilFrameFuncErrors(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("a")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Set", func() { err = pair.Standard.Set(nil) })
	if err == nil {
		t.Error("expected error when setting a nil frame func")
	}
}

func TestWrite_ClearsAndRedrawsSpinner(t *testing.T) {
	main := &syncBuffer{}
	spinny := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), main, spinny, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinny.Stop() })

	callWithTimeout(t, 2*time.Second, "Set", func() { err = pair.Spinny.Set(staticFrame([]byte("*"))) })
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if spinny.Len() == 0 {
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

	got := spinny.String()
	if !strings.Contains(got, string(clearBytes)) {
		t.Errorf("expected clear sequence on spinny stream, got %q", got)
	}
	if !strings.HasSuffix(got, "*") {
		t.Errorf("expected spinner to be redrawn after a newline-terminated write, got %q", got)
	}
}

func TestWrite_NoRedrawWithoutTrailingNewline(t *testing.T) {
	main := &syncBuffer{}
	spinny := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), main, spinny, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinny.Stop() })

	callWithTimeout(t, 2*time.Second, "Set", func() { err = pair.Spinny.Set(staticFrame([]byte("*"))) })
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	spinny.Reset()

	if _, err := pair.Standard.Write([]byte("partial")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := spinny.String(); got != string(clearBytes) {
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
	spinny := &failAfterWriter{n: 1, err: writeErr}
	main := &syncBuffer{}

	pair, err := WrapPair(context.Background(), main, spinny, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
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
	case gotErr := <-pair.Err():
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
	defer pair.Close()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForCondition(t, func() bool { return strings.Contains(w.String(), "*") })

	if _, err := pair.Spinny.Write([]byte("partial")); err != nil {
		t.Fatalf("expected payload write to succeed despite clear failure, got %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	if _, err := pair.Spinny.Write([]byte("hello\n")); err != nil {
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
	defer pair.Close()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForCondition(t, func() bool { return strings.Contains(w.String(), "*") })
	waitForCondition(t, func() bool { return countMarker() == baseline+1 })

	_, _ = pair.Spinny.Write([]byte("hello\n"))

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
	defer pair.Close()

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForCondition(t, func() bool { return strings.Contains(w.String(), "*") })

	if _, err := pair.Spinny.Write([]byte("hi\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case gotErr := <-pair.Err():
		if !errors.Is(gotErr, drawErr) {
			t.Errorf("expected the reported error to wrap the draw failure, got %v", gotErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected the draw failure to be reported on Err()")
	}
}

func TestTickerUpdatesFrame(t *testing.T) {
	spinny := &syncBuffer{}
	ticker := make(chan time.Time)
	frames := [][]byte{[]byte("a"), []byte("b"), []byte("c")}
	idx := 0
	frameFn := func() ([]byte, error) {
		f := frames[idx%len(frames)]
		idx++
		return f, nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, frameFn, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinny.Stop() })

	ticker <- time.Now()
	waitForCondition(t, func() bool { return strings.Contains(spinny.String(), "b") })

	ticker <- time.Now()
	waitForCondition(t, func() bool { return strings.Contains(spinny.String(), "c") })
}

func TestTicker_FrameErrorSkipsUpdate(t *testing.T) {
	spinny := &syncBuffer{}
	ticker := make(chan time.Time)
	var fail atomic.Bool
	frameFn := func() ([]byte, error) {
		if fail.Load() {
			return nil, errors.New("temporary failure")
		}
		return []byte("ok"), nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, frameFn, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinny.Stop() })

	fail.Store(true)
	ticker <- time.Now()
	time.Sleep(20 * time.Millisecond)

	fail.Store(false)
	ticker <- time.Now()
	waitForCondition(t, func() bool { return strings.Contains(spinny.String(), "ok") })
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
	defer callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

	ticker <- time.Now()
	time.Sleep(50 * time.Millisecond)

	if got := calls.Load(); got != 0 {
		t.Errorf("expected getFrame not to be called for a tick while not running, got %d call(s)", got)
	}
}

func TestStart_CtxCancellationStopsSpinner(t *testing.T) {
	spinny := &syncBuffer{}
	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(ctx) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	cancel()

	waitForCondition(t, func() bool { return strings.HasSuffix(spinny.String(), string(clearBytes)) })
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

	_ = pair.Standard.Start(context.Background())
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

func TestSetGetFrameWriteFailureThenCloseHelperProcess(t *testing.T) {
	if os.Getenv("SPINQ_DOUBLE_CLOSE_HELPER") != "1" {
		t.Skip("only runs as a subprocess helper; see TestSetGetFrame_WriteFailureThenCloseDoesNotCrashProcess")
	}

	spinny := &failAfterWriter{n: 1, err: errors.New("write boom")}
	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		fmt.Fprintln(os.Stderr, "WrapPair:", err)
		os.Exit(1)
	}

	if err := pair.Spinny.Start(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "Start:", err)
		os.Exit(1)
	}

	_ = pair.Spinny.Set(staticFrame([]byte("different")))
	pair.Close()
	fmt.Println("SURVIVED")
}

func TestSetGetFrame_WriteFailureThenCloseDoesNotCrashProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSetGetFrameWriteFailureThenCloseHelperProcess$", "-test.v")
	cmd.Env = append(os.Environ(), "SPINQ_DOUBLE_CLOSE_HELPER=1")
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("subprocess did not exit within the timeout, output:\n%s", out)
	}
	if err != nil {
		t.Errorf("expected a write failure in Set() followed by Close() not to crash the process: %v\noutput:\n%s", err, out)
		return
	}
	if !strings.Contains(string(out), "SURVIVED") {
		t.Errorf("expected the process to survive Set() + Close(), output:\n%s", out)
	}
}
