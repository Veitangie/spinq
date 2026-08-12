// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq

import (
	"bytes"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func (b *syncBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

type errWriter struct{ err error }

func (w errWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

type flakyWriter struct {
	n   atomic.Int64
	err error
}

func (w *flakyWriter) Write(p []byte) (int, error) {
	if w.n.Add(1)%2 == 0 {
		return 0, w.err
	}
	return len(p), nil
}

type failAfterWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
	n   int
	err error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.n <= 0 {
		return 0, w.err
	}
	w.n--
	return w.buf.Write(p)
}

type failOnCallWriter struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	call int
	on   int
	err  error
}

func (w *failOnCallWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.call++
	if w.call == w.on {
		return 0, w.err
	}
	return w.buf.Write(p)
}

func (w *failOnCallWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func staticFrame(b []byte) FrameFunc {
	return func() ([]byte, error) {
		return b, nil
	}
}

func asReal(t *testing.T, sw SpinqWriter) SpinqWriterReal {
	t.Helper()
	real, ok := sw.(SpinqWriterReal)
	if !ok {
		t.Fatalf("expected SpinqWriterReal, got %T", sw)
	}
	return real
}

func errorFrame(err error) FrameFunc {
	return func() ([]byte, error) {
		return nil, err
	}
}

func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool { //nolint:unparam // kept general-purpose even though every current caller passes the same value
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func dumpGoroutines() string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return string(buf[:n])
		}
		buf = make([]byte, 2*len(buf))
	}
}

func callWithTimeout(t *testing.T, timeout time.Duration, name string, fn func()) { //nolint:unparam // kept general-purpose even though every current caller passes the same value
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("%s did not return within %s (deadlocked) — goroutine dump:\n%s", name, timeout, dumpGoroutines())
	}
}

func waitForCondition(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !cond() {
		t.Fatal("condition not met within timeout")
	}
}
