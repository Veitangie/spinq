// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package spinq

import (
	"context"
	"os"
	"time"
)

// DefaultGetWidth is the Windows counterpart to resize_unix.go's
// version: there's no SIGWINCH equivalent, so it drives DefaultSigwinch's
// polling analogue (SigwinchFromPoller) instead of a real signal. A nil ctx
// defaults to context.Background(). Returns an error (falling both
// WithDefaultResizeDetection and WrapWithDefaultResizeDetection back to a
// no-op) whenever os.Stderr isn't a real console.
func DefaultGetWidth(ctx context.Context) (func() int, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	getWidth, err := WidthFromFile(os.Stderr)
	if err != nil {
		return nil, err
	}

	return CachedGetWidth(SigwinchFromPoller(ctx, 100*time.Millisecond), getWidth), nil
}

// WithDefaultResizeDetection enables resize detection for JustStart with
// zero configuration, using os.Stderr to size the terminal and polling
// (there's no SIGWINCH on Windows) to detect a resize. ctx governs the
// lifetime of the background poller; a nil ctx defaults to
// context.Background(). Falls back to a no-op if os.Stderr isn't a real
// console.
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
