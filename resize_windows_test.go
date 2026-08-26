// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package spinq

import (
	"context"
	"os"
	"testing"
	"time"
)

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

	getWidth, err := DefaultGetWidth(ctx)
	if err != nil {
		t.Fatalf("DefaultGetWidth: %v", err)
	}
	if getWidth == nil {
		t.Fatal("expected a non-nil getWidth on success")
	}
}

func TestDefaultResizeDetection_ErrorsWhenStderrIsNotATerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()
	withStderr(t, f)

	if _, err := DefaultGetWidth(context.Background()); err == nil {
		t.Error("expected an error when stderr is not a terminal")
	}
}

func TestDefaultResizeDetection_NilCtxDefaultsToBackground(t *testing.T) {
	withStderr(t, openTestPTY(t))

	if _, err := DefaultGetWidth(nil); err != nil { //nolint:staticcheck
		t.Fatalf("DefaultGetWidth(nil): %v", err)
	}
}

func TestWithDefaultResizeDetection_WiresOptionsWhenAvailable(t *testing.T) {
	withStderr(t, openTestPTY(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opt := WithDefaultResizeDetection(ctx)
	jso := opt(JustStartOptions{})
	if jso.GetWidth == nil {
		t.Error("expected WithDefaultResizeDetection to wire GetWidth when stderr is a terminal")
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
	if jso.GetWidth != nil {
		t.Error("expected WithDefaultResizeDetection to be a no-op when stderr is not a terminal")
	}
}

func TestWrapWithDefaultResizeDetection_WiresOptionsWhenAvailable(t *testing.T) {
	withStderr(t, openTestPTY(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opt := WrapWithDefaultResizeDetection(ctx)
	wo := opt(WrapOptions{})
	if wo.GetWidth == nil {
		t.Error("expected WrapWithDefaultResizeDetection to wire GetWidth when stderr is a terminal")
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
	if wo.GetWidth != nil {
		t.Error("expected WrapWithDefaultResizeDetection to be a no-op when stderr is not a terminal")
	}
}

func TestDefaultResizeDetectionOption_SucceedsWhenStderrIsATerminal(t *testing.T) {
	withStderr(t, openTestPTY(t))

	opt, getWidth, err := DefaultResizeDetection(context.Background())
	if err != nil {
		t.Fatalf("DefaultResizeDetection: %v", err)
	}
	if opt == nil {
		t.Fatal("expected a non-nil JustStartOptionsFunc on success")
	}
	if getWidth == nil {
		t.Fatal("expected a non-nil getWidth on success")
	}

	jso := opt(JustStartOptions{})
	if jso.GetWidth == nil {
		t.Fatal("expected the returned JustStartOptionsFunc to wire GetWidth")
	}
	if jso.GetWidth() != getWidth() {
		t.Errorf("expected the wired GetWidth and the returned getWidth to agree, got %d vs %d", jso.GetWidth(), getWidth())
	}
}

func TestDefaultResizeDetectionOption_FailureReturnsSafeNoopAndNoWidthToDetect(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()
	withStderr(t, f)

	opt, getWidth, err := DefaultResizeDetection(context.Background())
	if err == nil {
		t.Fatal("expected an error when stderr is not a terminal")
	}
	if opt == nil {
		t.Fatal("expected a non-nil (no-op) JustStartOptionsFunc on failure")
	}
	if getWidth == nil {
		t.Fatal("expected a non-nil getWidth on failure")
	}
	if got := getWidth(); got != -1 {
		t.Errorf("expected the failure-path getWidth to report -1 (nothing to detect), got %d", got)
	}
	if jso := opt(JustStartOptions{}); jso.GetWidth != nil {
		t.Error("expected the failure-path JustStartOptionsFunc to be a no-op, leaving GetWidth unset")
	}
}

func TestDefaultResizeDetectionOption_FailurePassedToJustStartDoesNotPanic(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()
	withStderr(t, f)

	opt, _, err := DefaultResizeDetection(context.Background())
	if err == nil {
		t.Fatal("expected an error when stderr is not a terminal")
	}

	panicked := func() (p any) {
		defer func() { p = recover() }()
		_, _ = JustStart(opt)
		return nil
	}()

	if panicked != nil {
		t.Errorf("passing DefaultResizeDetection's failure-path JustStartOptionsFunc into JustStart panicked "+
			"instead of behaving as a harmless no-op: %v", panicked)
	}
}

func TestWrapDefaultResizeDetectionOption_SucceedsWhenStderrIsATerminal(t *testing.T) {
	withStderr(t, openTestPTY(t))

	opt, getWidth, err := WrapDefaultResizeDetection(context.Background())
	if err != nil {
		t.Fatalf("WrapDefaultResizeDetection: %v", err)
	}
	if opt == nil {
		t.Fatal("expected a non-nil WrapOptionsFunc on success")
	}
	if getWidth == nil {
		t.Fatal("expected a non-nil getWidth on success")
	}

	wo := opt(WrapOptions{})
	if wo.GetWidth == nil {
		t.Fatal("expected the returned WrapOptionsFunc to wire GetWidth")
	}
	if wo.GetWidth() != getWidth() {
		t.Errorf("expected the wired GetWidth and the returned getWidth to agree, got %d vs %d", wo.GetWidth(), getWidth())
	}
}

func TestWrapDefaultResizeDetectionOption_FailureReturnsSafeNoopAndNoWidthToDetect(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()
	withStderr(t, f)

	opt, getWidth, err := WrapDefaultResizeDetection(context.Background())
	if err == nil {
		t.Fatal("expected an error when stderr is not a terminal")
	}
	if opt == nil {
		t.Fatal("expected a non-nil (no-op) WrapOptionsFunc on failure")
	}
	if getWidth == nil {
		t.Fatal("expected a non-nil getWidth on failure")
	}
	if got := getWidth(); got != -1 {
		t.Errorf("expected the failure-path getWidth to report -1 (nothing to detect), got %d", got)
	}
	if wo := opt(WrapOptions{}); wo.GetWidth != nil {
		t.Error("expected the failure-path WrapOptionsFunc to be a no-op, leaving GetWidth unset")
	}
}

func TestWrapDefaultResizeDetectionOption_FailurePassedToWrapPairDoesNotPanic(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()
	withStderr(t, f)

	opt, _, err := WrapDefaultResizeDetection(context.Background())
	if err == nil {
		t.Fatal("expected an error when stderr is not a terminal")
	}

	panicked := func() (p any) {
		defer func() { p = recover() }()
		_, _ = WrapPair(context.Background(), &syncBuffer{}, &syncBuffer{}, staticFrame([]byte("*")), make(chan time.Time), opt)
		return nil
	}()

	if panicked != nil {
		t.Errorf("passing WrapDefaultResizeDetection's failure-path WrapOptionsFunc into WrapPair panicked "+
			"instead of behaving as a harmless no-op (like WrapWithDefaultResizeDetection's failure path does): %v", panicked)
	}
}
