// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentWrites(t *testing.T) {
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

	const goroutines = 20
	const writesEach = 50

	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			line := fmt.Appendf(nil, "line-%d\n", id)
			for range writesEach {
				if _, err := pair.Standard.Write(line); err != nil {
					t.Errorf("write error: %v", err)
				}
			}
		}(g)
	}
	if !waitTimeout(&wg, 5*time.Second) {
		t.Fatalf("concurrent writes deadlocked instead of completing — goroutine dump:\n%s", dumpGoroutines())
	}

	wantLines := goroutines * writesEach
	if gotLines := strings.Count(main.String(), "\n"); gotLines != wantLines {
		t.Errorf("expected %d complete lines, got %d", wantLines, gotLines)
	}
}

func TestConcurrentSetFrameAndWrite(t *testing.T) {
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
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinner.Stop() })

	frames := [][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d")}

	var wg sync.WaitGroup
	for i := range 100 {
		frame := frames[i%len(frames)]

		wg.Go(func() {
			if err := pair.Spinner.SetFrame(staticFrame(frame)); err != nil {
				t.Errorf("set error: %v", err)
			}
		})
		wg.Go(func() {
			if _, err := pair.Standard.Write([]byte("x\n")); err != nil {
				t.Errorf("write error: %v", err)
			}
		})
	}
	if !waitTimeout(&wg, 5*time.Second) {
		t.Fatalf("concurrent SetFrame/Write deadlocked instead of completing — goroutine dump:\n%s", dumpGoroutines())
	}
}

func TestConcurrentTicksAndWrites(t *testing.T) {
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

	stopTicks := make(chan struct{})
	tickerDone := make(chan struct{})
	go func() {
		defer close(tickerDone)
		for {
			select {
			case ticker <- time.Now():
			case <-stopTicks:
				return
			}
		}
	}()

	var ops sync.WaitGroup
	ops.Go(func() {
		for range 200 {
			if _, err := pair.Standard.Write([]byte("tick\n")); err != nil {
				t.Errorf("write error: %v", err)
			}
		}
	})
	ops.Go(func() {
		for i := range 200 {
			if i%2 == 0 {
				_ = pair.Spinner.SetTicker(make(chan time.Time))
			} else {
				_ = pair.Spinner.SetTicker(ticker)
			}
		}
		_ = pair.Spinner.SetTicker(ticker)
	})

	ops.Wait()
	close(stopTicks)
	<-tickerDone

	var stopErr error
	callWithTimeout(t, 2*time.Second, "Stop", func() { stopErr = pair.Spinner.Stop() })
	if stopErr != nil {
		t.Fatalf("stop: %v", stopErr)
	}
}

func runRecovered(f func()) (recovered any) {
	defer func() { recovered = recover() }()
	f()
	return nil
}

func TestConcurrentStartStop(t *testing.T) {
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	ctx := context.Background()

	var panics int32
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if r := runRecovered(func() { _ = pair.Spinner.Start(ctx) }); r != nil {
				atomic.AddInt32(&panics, 1)
			}
		})
		wg.Go(func() {
			if r := runRecovered(func() { _ = pair.Spinner.Stop() }); r != nil {
				atomic.AddInt32(&panics, 1)
			}
		})
	}

	if !waitTimeout(&wg, 5*time.Second) {
		t.Fatalf("concurrent Start/Stop deadlocked instead of completing — goroutine dump:\n%s", dumpGoroutines())
	}

	if panics > 0 {
		t.Errorf("concurrent Start/Stop panicked %d times", panics)
	}
}

func TestConcurrentStartStopWithWrites(t *testing.T) {
	main := &syncBuffer{}
	spinner := &syncBuffer{}

	pair, err := WrapPair(context.Background(), main, spinner, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	ctx := context.Background()

	var panics int32
	var wg sync.WaitGroup
	for range 30 {
		wg.Go(func() {
			if r := runRecovered(func() { _ = pair.Spinner.Start(ctx) }); r != nil {
				atomic.AddInt32(&panics, 1)
			}
		})
		wg.Go(func() {
			if r := runRecovered(func() { _ = pair.Spinner.Stop() }); r != nil {
				atomic.AddInt32(&panics, 1)
			}
		})
		wg.Go(func() {
			if _, err := pair.Standard.Write([]byte("x\n")); err != nil {
				t.Errorf("write error: %v", err)
			}
		})
	}

	if !waitTimeout(&wg, 5*time.Second) {
		t.Fatalf("concurrent Start/Stop deadlocked instead of completing — goroutine dump:\n%s", dumpGoroutines())
	}

	if panics > 0 {
		t.Errorf("concurrent Start/Stop panicked %d times", panics)
	}
}

func TestConcurrentTicks_OverlappingFetchesSkipWhileOneIsInFlight(t *testing.T) {
	spinner := &syncBuffer{}
	var calls atomic.Int32
	release := make(chan struct{})
	inner := Simple([]string{"a", "b", "c", "d", "e"})

	frameFn := func() ([]byte, error) {
		if calls.Add(1) == 1 {
			return inner()
		}
		<-release
		return inner()
	}

	ticker := make(chan time.Time)
	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, frameFn, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	const ticks = 30
	for range ticks {
		ticker <- time.Now()
	}

	time.Sleep(200 * time.Millisecond)
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected the 29 overlapping ticks after the first to be skipped while a fetch is already in flight (2 calls total: Start's + the one in-flight ticker fetch), got %d calls", got)
	}

	close(release)

	waitForCondition(t, func() bool { return strings.Contains(spinner.String(), "b") })

	ticker <- time.Now()
	waitForCondition(t, func() bool { return calls.Load() == 3 })

	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })
}

func TestSetFrame_DoesNotApplyStaleResultFromReplacedGetFrame(t *testing.T) {
	spinner := &syncBuffer{}
	var oldCalls atomic.Int32
	oldRelease := make(chan struct{})

	oldGetFrame := func() ([]byte, error) {
		if oldCalls.Add(1) == 1 {
			return []byte("old-fast"), nil
		}
		<-oldRelease
		return []byte("OLD-STALE"), nil
	}

	ticker := make(chan time.Time)
	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinner, oldGetFrame, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	ticker <- time.Now()
	time.Sleep(100 * time.Millisecond)

	setDone := make(chan struct{})
	go func() {
		defer close(setDone)
		if err := pair.Spinner.SetFrame(staticFrame([]byte("old-fast"))); err != nil {
			t.Errorf("SetFrame: %v", err)
		}
	}()

	close(oldRelease)

	select {
	case <-setDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("SetFrame did not return within 2s (deadlocked) — goroutine dump:\n%s", dumpGoroutines())
	}

	time.Sleep(150 * time.Millisecond)

	if got := spinner.String(); !strings.HasSuffix(got, "old-fast") {
		t.Errorf("display did not converge on the new getFrame's output after the swap: %q", got)
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })
}

func TestErrCh_WindDownRacesTickerGoroutinePanicReport(t *testing.T) {
	for range 100 {
		entered := make(chan struct{})
		release := make(chan struct{})
		var callNum atomic.Int32
		frame := func() ([]byte, error) {
			if callNum.Add(1) == 2 {
				close(entered)
				<-release
				panic("boom")
			}
			return []byte("*"), nil
		}

		ticker := make(chan time.Time)
		pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, frame, ticker)
		if err != nil {
			t.Fatalf("WrapPair: %v", err)
		}
		callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
		if err != nil {
			t.Fatalf("start: %v", err)
		}

		callWithTimeout(t, 2*time.Second, "tick send", func() { ticker <- time.Now() })
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("tick-triggered fetch never reached the gate")
		}

		close(release)
		callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })
	}
}

func TestFrameFunc_NeverCalledConcurrently_SlowTickInFlightAcrossStopStart(t *testing.T) {
	var inCall atomic.Bool
	var concurrentCallDetected atomic.Bool
	var callNum atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})

	frame := func() ([]byte, error) {
		if !inCall.CompareAndSwap(false, true) {
			concurrentCallDetected.Store(true)
			return []byte("!CONCURRENT!"), nil
		}
		defer inCall.Store(false)

		if callNum.Add(1) == 2 {
			close(entered)
			<-release
		}
		return []byte("*"), nil
	}

	ticker := make(chan time.Time)
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, frame, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	ticker <- time.Now()
	<-entered

	callWithTimeout(t, 2*time.Second, "Stop", func() {
		if err := pair.Spinner.Stop(); err != nil {
			t.Errorf("stop: %v", err)
		}
	})

	startDone := make(chan error, 1)
	go func() { startDone <- pair.Spinner.Start(context.Background()) }()

	time.Sleep(100 * time.Millisecond)
	close(release)

	select {
	case err := <-startDone:
		if err != nil {
			t.Fatalf("restart: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after the in-flight fetch was released")
	}

	if concurrentCallDetected.Load() {
		t.Error("FrameFunc was called concurrently with itself across a Stop+Start while a tick-triggered fetch was still in flight - violates the FrameFunc \"never called concurrently\" contract")
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })
}

func TestSetFrame_RacesConcurrentFetchOfSameUnderlyingClosure(t *testing.T) {
	var callCount atomic.Int32
	var counter int
	gate := make(chan struct{})

	getFrame := func() ([]byte, error) { //nolint:unparam
		if callCount.Add(1) == 1 {
			counter++
			return fmt.Appendf(nil, "%d", counter), nil
		}
		<-gate
		counter++
		return fmt.Appendf(nil, "%d", counter), nil
	}

	ticker := make(chan time.Time)
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, getFrame, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	ticker <- time.Now()
	time.Sleep(50 * time.Millisecond)

	setDone := make(chan struct{})
	go func() {
		defer close(setDone)
		if err := pair.Spinner.SetFrame(getFrame); err != nil {
			t.Errorf("SetFrame: %v", err)
		}
	}()
	time.Sleep(50 * time.Millisecond)

	close(gate)

	select {
	case <-setDone:
	case <-time.After(2 * time.Second):
		t.Fatal("SetFrame did not return")
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })
}

func TestStart_ClosedWhileResponsePendingReturnsErrClosed(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	frame := func() ([]byte, error) {
		close(entered)
		<-release
		return []byte("*"), nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, frame, make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	startDone := make(chan error, 1)
	go func() { startDone <- pair.Spinner.Start(context.Background()) }()
	<-entered

	closeDone := make(chan struct{})
	go func() { _ = pair.Spinner.Close(); close(closeDone) }()

	select {
	case err := <-startDone:
		if !errors.Is(err, ErrClosed) {
			t.Errorf("expected Start to return ErrClosed once the spinner closes while its response is still pending, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after the spinner was closed concurrently")
	}

	close(release)
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after the in-flight fetch unblocked")
	}
}

func TestStopNoClear_ClosedWhileResponsePendingReturnsErrClosed(t *testing.T) {
	var callCount atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	frame := func() ([]byte, error) {
		if callCount.Add(1) == 1 {
			return []byte("*"), nil
		}
		close(entered)
		<-release
		return []byte("*"), nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, frame, make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	stopDone := make(chan error, 1)
	go func() { stopDone <- pair.Spinner.StopNoClear("") }()
	<-entered

	closeDone := make(chan struct{})
	go func() { _ = pair.Spinner.Close(); close(closeDone) }()

	select {
	case err := <-stopDone:
		if !errors.Is(err, ErrClosed) {
			t.Errorf("expected StopNoClear to return ErrClosed once the spinner closes while its response is still pending, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StopNoClear did not return after the spinner was closed concurrently")
	}

	close(release)
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after the in-flight fetch unblocked")
	}
}

func TestSetFrame_ClosedWhileResponsePendingReturnsErrClosed(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	blockingFrame := func() ([]byte, error) { //nolint:unparam
		close(entered)
		<-release
		return []byte("new"), nil
	}

	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinner.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	setDone := make(chan error, 1)
	go func() { setDone <- pair.Spinner.SetFrame(blockingFrame) }()
	<-entered

	closeDone := make(chan struct{})
	go func() { _ = pair.Spinner.Close(); close(closeDone) }()

	select {
	case err := <-setDone:
		if !errors.Is(err, ErrClosed) {
			t.Errorf("expected SetFrame to return ErrClosed once the spinner closes while its response is still pending, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SetFrame did not return after the spinner was closed concurrently")
	}

	close(release)
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after the in-flight fetch unblocked")
	}
}

func TestStress_1000GoroutinesMixedOperationsWithFaultyInputs(t *testing.T) {
	var frameCalls atomic.Int64
	frame := func() ([]byte, error) {
		switch frameCalls.Add(1) % 7 {
		case 0:
			return nil, errors.New("stress: transient frame error")
		case 1:
			panic("stress: deliberate frame panic")
		default:
			return []byte("*"), nil
		}
	}

	var widthCalls atomic.Int64
	getWidth := func() int {
		switch widthCalls.Add(1) % 5 {
		case 0:
			return -1000
		case 1:
			return 0
		case 2:
			return 1 << 30
		default:
			return 80
		}
	}

	ticker := make(chan time.Time)
	pair, err := WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, frame, ticker, WrapWithResizeDetection(getWidth))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	stopTicks := make(chan struct{})
	tickerDone := make(chan struct{})
	go func() {
		defer close(tickerDone)
		for {
			select {
			case ticker <- time.Now():
			case <-stopTicks:
				return
			}
		}
	}()

	const goroutines = 1000
	ctx := context.Background()
	var panicsEscaped atomic.Int64

	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					panicsEscaped.Add(1)
				}
			}()
			switch i % 7 {
			case 0:
				_ = pair.Spinner.Start(ctx)
			case 1:
				_ = pair.Spinner.Stop()
			case 2:
				_ = pair.Spinner.SetFrame(frame)
			case 3:
				_, _ = pair.Standard.Write([]byte("x\n"))
			case 4:
				_ = pair.Spinner.StopNoClear("bye")
			case 5:
				_ = pair.Spinner.StopWith("done")
			case 6:
				if i%2 == 0 {
					_ = pair.Spinner.SetTicker(nil)
				} else {
					_ = pair.Spinner.SetTicker(ticker)
				}
			}
		})
	}

	deadlocked := !waitTimeout(&wg, 15*time.Second)
	close(stopTicks)
	<-tickerDone

	if deadlocked {
		t.Fatalf("1000 concurrent mixed operations (with faulty inputs and a panicking FrameFunc) deadlocked instead of completing — goroutine dump:\n%s", dumpGoroutines())
	}
	if got := panicsEscaped.Load(); got > 0 {
		t.Errorf("expected no panic to ever escape a public API call regardless of what the FrameFunc does internally, but %d did", got)
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { _ = pair.Spinner.Close() })
}

func TestConcurrentStopNoClearVsFailingWrite_NoDataRace(t *testing.T) {
	w := &flakyWriter{err: errors.New("boom")}
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

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = pair.Spinner.Write([]byte("x\n"))
			}
		}
	})

	for range 2000 {
		_ = pair.Spinner.StopNoClear("")
		_ = pair.Spinner.Start(context.Background())
	}

	close(stop)
	if !waitTimeout(&wg, 5*time.Second) {
		t.Fatalf("writer goroutine did not finish — goroutine dump:\n%s", dumpGoroutines())
	}
}
