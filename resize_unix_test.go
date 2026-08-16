// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package spinq

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestDefaultSigwinch_FiresOnRealSignal(t *testing.T) {
	ctx := t.Context()
	sigwinch := DefaultSigwinch(ctx)

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	select {
	case _, ok := <-sigwinch:
		if !ok {
			t.Fatal("expected a signal, got a closed channel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SIGWINCH to be forwarded")
	}
}

func TestDefaultSigwinch_ClosesAndStopsSignalOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sigwinch := DefaultSigwinch(ctx)
	cancel()
	drainUntilClosed(t, sigwinch, 2*time.Second)
}

type fakeSignal string

func (f fakeSignal) String() string { return string(f) }
func (f fakeSignal) Signal()        {}

func TestSigwinchFromOs_FiltersToSigwinchOnly(t *testing.T) {
	in := make(chan os.Signal, 4)
	sigwinch := SigwinchFromOs(in)

	in <- fakeSignal("not sigwinch")
	in <- syscall.SIGWINCH
	in <- fakeSignal("also not sigwinch")
	close(in)

	count := drainUntilClosed(t, sigwinch, 2*time.Second)
	if count != 1 {
		t.Errorf("expected exactly 1 forwarded value (only for SIGWINCH), got %d", count)
	}
}

func TestSigwinchFromOs_DoesNotBlockWhenBufferIsFull(t *testing.T) {
	in := make(chan os.Signal, 4)
	sigwinch := SigwinchFromOs(in)

	in <- syscall.SIGWINCH
	waitForCondition(t, func() bool { return len(sigwinch) == 1 })
	in <- syscall.SIGWINCH
	in <- syscall.SIGWINCH
	close(in)

	drainUntilClosed(t, sigwinch, 2*time.Second)
}

func withStderr(t *testing.T, f *os.File) {
	t.Helper()
	orig := os.Stderr
	os.Stderr = f
	t.Cleanup(func() { os.Stderr = orig })
}

func TestDefaultResizeDetection_SucceedsWhenStderrIsATerminal(t *testing.T) {
	withStderr(t, openTestPTY(t))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigwinCh, getWidth, err := defaultResizeDetection(ctx)
	if err != nil {
		t.Fatalf("defaultResizeDetection: %v", err)
	}
	if sigwinCh == nil || getWidth == nil {
		t.Fatal("expected non-nil sigwinCh and getWidth on success")
	}
}

func TestDefaultResizeDetection_ErrorsWhenStderrIsNotATerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()
	withStderr(t, f)

	if _, _, err := defaultResizeDetection(context.Background()); err == nil {
		t.Error("expected an error when stderr is not a terminal")
	}
}

func TestDefaultResizeDetection_NilCtxDefaultsToBackground(t *testing.T) {
	withStderr(t, openTestPTY(t))

	if _, _, err := defaultResizeDetection(nil); err != nil { //nolint:staticcheck
		t.Fatalf("defaultResizeDetection(nil): %v", err)
	}
}

func TestWithDefaultResizeDetection_WiresOptionsWhenAvailable(t *testing.T) {
	withStderr(t, openTestPTY(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opt := WithDefaultResizeDetection(ctx)
	jso := opt(JustStartOptions{})
	if jso.SigwinCh == nil || jso.GetWidth == nil {
		t.Error("expected WithDefaultResizeDetection to wire SigwinCh/GetWidth when stderr is a terminal")
	}
}

func TestWithDefaultResizeDetection_NoopWhenUnavailable(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()
	withStderr(t, f)

	opt := WithDefaultResizeDetection(context.Background())
	jso := opt(JustStartOptions{})
	if jso.SigwinCh != nil || jso.GetWidth != nil {
		t.Error("expected WithDefaultResizeDetection to be a no-op when stderr is not a terminal")
	}
}

func TestWrapWithDefaultResizeDetection_WiresOptionsWhenAvailable(t *testing.T) {
	withStderr(t, openTestPTY(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opt := WrapWithDefaultResizeDetection(ctx)
	wo := opt(WrapOptions{})
	if wo.SigwinCh == nil || wo.GetWidth == nil {
		t.Error("expected WrapWithDefaultResizeDetection to wire SigwinCh/GetWidth when stderr is a terminal")
	}
}

func TestWrapWithDefaultResizeDetection_NoopWhenUnavailable(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()
	withStderr(t, f)

	opt := WrapWithDefaultResizeDetection(context.Background())
	wo := opt(WrapOptions{})
	if wo.SigwinCh != nil || wo.GetWidth != nil {
		t.Error("expected WrapWithDefaultResizeDetection to be a no-op when stderr is not a terminal")
	}
}
