// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentWrites(t *testing.T) {
	main := &syncBuffer{}
	spinny := &syncBuffer{}
	ticker := make(chan time.Time)

	pair, err := WrapPair(context.Background(), main, spinny, staticFrame([]byte("*")), ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Standard.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Standard.Stop() })

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

func TestConcurrentSetAndWrite(t *testing.T) {
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
	defer callWithTimeout(t, 2*time.Second, "Stop", func() { _ = pair.Spinny.Stop() })

	frames := [][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d")}

	var wg sync.WaitGroup
	for i := range 100 {
		frame := frames[i%len(frames)]

		wg.Go(func() {
			if err := pair.Spinny.Set(staticFrame(frame)); err != nil {
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
		t.Fatalf("concurrent Set/Write deadlocked instead of completing — goroutine dump:\n%s", dumpGoroutines())
	}
}

func TestConcurrentTicksAndWrites(t *testing.T) {
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

	var writesDone sync.WaitGroup
	writesDone.Go(func() {
		for range 200 {
			if _, err := pair.Standard.Write([]byte("tick\n")); err != nil {
				t.Errorf("write error: %v", err)
			}
		}
	})

	writesDone.Wait()
	close(stopTicks)
	<-tickerDone

	var stopErr error
	callWithTimeout(t, 2*time.Second, "Stop", func() { stopErr = pair.Spinny.Stop() })
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
			if r := runRecovered(func() { _ = pair.Standard.Start(ctx) }); r != nil {
				atomic.AddInt32(&panics, 1)
			}
		})
		wg.Go(func() {
			if r := runRecovered(func() { _ = pair.Standard.Stop() }); r != nil {
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
	spinny := &syncBuffer{}

	pair, err := WrapPair(context.Background(), main, spinny, staticFrame([]byte("*")), make(chan time.Time))
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}

	ctx := context.Background()

	var panics int32
	var wg sync.WaitGroup
	for range 30 {
		wg.Go(func() {
			if r := runRecovered(func() { _ = pair.Standard.Start(ctx) }); r != nil {
				atomic.AddInt32(&panics, 1)
			}
		})
		wg.Go(func() {
			if r := runRecovered(func() { _ = pair.Standard.Stop() }); r != nil {
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

func TestConcurrentTicks_CoalesceOverlappingFetchesForSameRevision(t *testing.T) {
	spinny := &syncBuffer{}
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
	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, frameFn, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	const ticks = 30
	for range ticks {
		ticker <- time.Now()
	}

	time.Sleep(200 * time.Millisecond)
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected the 30 overlapping ticks to coalesce onto the one in-flight fetch (2 calls total: Start's + one shared), got %d calls", got)
	}

	close(release)

	waitForCondition(t, func() bool { return strings.Contains(spinny.String(), "b") })

	ticker <- time.Now()
	waitForCondition(t, func() bool { return calls.Load() == 3 })

	callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })
}

func TestSetGetFrame_DoesNotApplyStaleResultFromReplacedGetFrame(t *testing.T) {
	spinny := &syncBuffer{}
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
	pair, err := WrapPair(context.Background(), &syncBuffer{}, spinny, oldGetFrame, ticker)
	if err != nil {
		t.Fatalf("WrapPair: %v", err)
	}
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	ticker <- time.Now()
	time.Sleep(100 * time.Millisecond)

	setDone := make(chan struct{})
	go func() {
		defer close(setDone)
		if err := pair.Spinny.Set(staticFrame([]byte("old-fast"))); err != nil {
			t.Errorf("Set: %v", err)
		}
	}()

	close(oldRelease)

	select {
	case <-setDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("Set did not return within 2s (deadlocked) — goroutine dump:\n%s", dumpGoroutines())
	}

	time.Sleep(150 * time.Millisecond)

	if got := spinny.String(); !strings.HasSuffix(got, "old-fast") {
		t.Errorf("display did not converge on the new getFrame's output after the swap: %q", got)
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })
}

func TestSetGetFrame_RacesConcurrentFetchOfSameUnderlyingClosure(t *testing.T) {
	var callCount atomic.Int32
	var counter int
	gate := make(chan struct{})

	getFrame := func() ([]byte, error) { //nolint:unparam // signature fixed by FrameFunc
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
	callWithTimeout(t, 2*time.Second, "Start", func() { err = pair.Spinny.Start(context.Background()) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	ticker <- time.Now()
	time.Sleep(50 * time.Millisecond)

	setDone := make(chan struct{})
	go func() {
		defer close(setDone)
		if err := pair.Spinny.Set(getFrame); err != nil {
			t.Errorf("Set: %v", err)
		}
	}()
	time.Sleep(50 * time.Millisecond)

	close(gate)

	select {
	case <-setDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Set did not return")
	}

	callWithTimeout(t, 2*time.Second, "Close", func() { pair.Close() })
}
