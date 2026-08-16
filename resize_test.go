// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestWidthFromFile_NilFile(t *testing.T) {
	if _, err := WidthFromFile(nil); err == nil {
		t.Error("expected an error for a nil file")
	}
}

func TestWidthFromFile_NotATerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := WidthFromFile(f); err == nil {
		t.Error("expected an error for a non-terminal file")
	}
}

func TestWidthFromFile_ReadsRealTerminalWidth(t *testing.T) {
	tty := openTestPTY(t)

	if err := pty.Setsize(tty, &pty.Winsize{Rows: 24, Cols: 77}); err != nil {
		t.Fatalf("Setsize: %v", err)
	}

	getWidth, err := WidthFromFile(tty)
	if err != nil {
		t.Fatalf("WidthFromFile: %v", err)
	}
	if got := getWidth(); got != 77 {
		t.Errorf("expected width 77, got %d", got)
	}
}

func TestWidthFromFile_ReturnsZeroAfterUnderlyingFileCloses(t *testing.T) {
	master, tty, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open: %v (platform without PTY support?)", err)
	}
	defer func() { _ = master.Close() }()

	getWidth, err := WidthFromFile(tty)
	if err != nil {
		t.Fatalf("WidthFromFile: %v", err)
	}
	_ = tty.Close()

	if got := getWidth(); got != 0 {
		t.Errorf("expected 0 once the underlying file is closed, got %d", got)
	}
}

func drainUntilClosed(t *testing.T, ch <-chan struct{}, timeout time.Duration) (count int) { //nolint:unparam
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return count
			}
			count++
		case <-deadline:
			t.Fatal("timed out waiting for the channel to close")
			return count
		}
	}
}

func TestSigwinchFromAny_ForwardsAndClosesWhenSourceCloses(t *testing.T) {
	in := make(chan any, 4)
	sigwinch := SigwinchFromAny(in)

	in <- struct{}{}
	select {
	case _, ok := <-sigwinch:
		if !ok {
			t.Fatal("expected a value, got a closed channel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a forwarded signal")
	}

	close(in)
	drainUntilClosed(t, sigwinch, 2*time.Second)
}

func TestSigwinchFromAny_DoesNotBlockWhenBufferIsFull(t *testing.T) {
	in := make(chan any, 4)
	sigwinch := SigwinchFromAny(in)

	in <- struct{}{}
	waitForCondition(t, func() bool { return len(sigwinch) == 1 })
	in <- struct{}{}
	in <- struct{}{}
	close(in)

	drainUntilClosed(t, sigwinch, 2*time.Second)
}

func TestSigwinchFromPoller_FiresPeriodicallyAndClosesOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sigwinch := SigwinchFromPoller(ctx, 5*time.Millisecond)

	select {
	case _, ok := <-sigwinch:
		if !ok {
			t.Fatal("expected a tick, got a closed channel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first poll tick")
	}

	cancel()
	drainUntilClosed(t, sigwinch, 2*time.Second)
}

func TestLiveGetWidth_InitialValueFromGetWidthUpfront(t *testing.T) {
	sigwinch := make(chan struct{})
	live := LiveGetWidth(sigwinch, func() int { return 80 })
	if got := live(); got != 80 {
		t.Errorf("expected initial width 80, got %d", got)
	}
}

func TestLiveGetWidth_UpdatesOnlyAfterSignal(t *testing.T) {
	sigwinch := make(chan struct{})
	width := &atomic.Int64{}
	width.Store(80)
	calls := &atomic.Int64{}
	live := LiveGetWidth(sigwinch, func() int {
		calls.Add(1)
		return int(width.Load())
	})

	width.Store(120)
	if got := live(); got != 80 {
		t.Errorf("expected the cached width 80 before any signal, got %d", got)
	}
	if calls.Load() != 1 {
		t.Errorf("expected exactly one real getWidth call (the upfront one), got %d", calls.Load())
	}

	sigwinch <- struct{}{}
	waitForCondition(t, func() bool { return live() == 120 })
}

func TestLiveGetWidth_SafeForConcurrentReads(t *testing.T) {
	sigwinch := make(chan struct{})
	live := LiveGetWidth(sigwinch, func() int { return 80 })

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			for range 100 {
				if got := live(); got != 80 {
					t.Errorf("expected 80, got %d", got)
				}
			}
		})
	}
	wg.Wait()
}
