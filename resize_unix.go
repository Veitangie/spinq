// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package spinq

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// DefaultSigwinch returns a <-chan struct{} that receives a value each time
// the process gets a real SIGWINCH, translating os.Signal's payload-bearing
// shape into spinq's plain "something changed" signal. ctx governs its
// lifetime: cancelling it stops the background goroutine, undoes the
// signal.Notify registration, and closes the returned channel. Like
// SigwinchFromPoller and unlike Every, that cleanup is not optional - an
// abandoned goroutine here is a live GC root that never gets collected.
func DefaultSigwinch(ctx context.Context) <-chan struct{} {
	realSigwinch := make(chan os.Signal, 1)
	signal.Notify(realSigwinch, syscall.SIGWINCH)
	sigwinch := make(chan struct{}, 1)
	go func() {
		defer signal.Stop(realSigwinch)
		defer close(sigwinch)
		for {
			select {
			case <-realSigwinch:
				select {
				case sigwinch <- struct{}{}:
				default:
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return sigwinch
}

// SigwinchFromOs adapts a caller-owned <-chan os.Signal into spinq's plain
// "something changed" shape, filtering out anything that isn't SIGWINCH.
// Use this if you already run your own signal.Notify for other signals and
// want to fold SIGWINCH handling into it, rather than giving spinq its own
// dedicated registration via DefaultSigwinch. The returned channel closes
// once in is closed; spinq never closes in itself, that's the caller's
// signal.Stop to make.
func SigwinchFromOs(in <-chan os.Signal) <-chan struct{} {
	sigwinch := make(chan struct{}, 1)
	go func() {
		defer close(sigwinch)
		for sig := range in {
			if sig == syscall.SIGWINCH {
				select {
				case sigwinch <- struct{}{}:
				default:
				}
			}
		}
	}()
	return sigwinch
}

// DefaultGetWidth is the shared core behind WithDefaultResizeDetection
// and WrapWithDefaultResizeDetection: os.Stderr sized via WidthFromFile, real
// SIGWINCH via DefaultSigwinch. A nil ctx defaults to context.Background().
// Returns an error whenever os.Stderr isn't a real terminal, which both
// callers turn into a no-op rather than propagating.
func DefaultGetWidth(ctx context.Context) (func() int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	getWidth, err := WidthFromFile(os.Stderr)
	if err != nil {
		return nil, err
	}

	return CachedGetWidth(DefaultSigwinch(ctx), getWidth), nil
}

// WithDefaultResizeDetection enables resize detection for JustStart with
// zero configuration, using os.Stderr to size the terminal and a real
// SIGWINCH handler to detect a resize. ctx governs the lifetime of that
// handler (see DefaultSigwinch); a nil ctx defaults to
// context.Background(). Falls back to a no-op if os.Stderr isn't a real
// terminal.
func WithDefaultResizeDetection(ctx context.Context) JustStartOptionsFunc {
	getWidth, err := DefaultGetWidth(ctx)
	if err != nil {
		return noop()
	}
	return WithResizeDetection(getWidth)
}

// WrapWithDefaultResizeDetection is WithDefaultResizeDetection for the
// lower-level WrapPair/WrapFilePair/WrapOS family.
func WrapWithDefaultResizeDetection(ctx context.Context) WrapOptionsFunc {
	getWidth, err := DefaultGetWidth(ctx)
	if err != nil {
		return func(wo WrapOptions) WrapOptions { return wo }
	}
	return WrapWithResizeDetection(getWidth)
}
