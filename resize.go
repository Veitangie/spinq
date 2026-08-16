// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

// LiveGetWidth wraps a (possibly expensive, e.g. syscall-backed) getWidth
// behind a cheap, freely shareable one: it calls the real getWidth once up
// front and again each time sigwinch signals, caching the result in an
// atomic that the returned func just reads. This is the one legitimate
// consumer of a raw sigwinch channel for width purposes - channels aren't
// broadcast, so only one reader can safely drain a given sigwinch, but many
// readers can safely call the func LiveGetWidth hands back.
//
// This is the piece that makes width-aware code cheap to call from a hot
// path: awareClearerDrawer's clear() (driven by WrapWithResizeDetection) and
// any number of Dynamic-built FrameFuncs can all share the *same*
// LiveGetWidth output and each just do an atomic read, rather than each
// hitting the real getWidth independently. Always prefer passing
// LiveGetWidth's output into Dynamic over a raw getWidth - Dynamic is called
// on every frame render, and a raw syscall-backed getWidth there reintroduces
// exactly the per-call cost this function exists to avoid.
func LiveGetWidth(sigwinch <-chan struct{}, getWidth func() int) func() int {
	current := atomic.Int64{}
	current.Store(int64(getWidth()))
	go func() {
		for range sigwinch {
			current.Store(int64(getWidth()))
		}
	}()
	return func() int {
		return int(current.Load())
	}
}

// WidthFromFile returns a getWidth func that queries file's terminal width
// on every call via a real syscall (x/term.GetSize) - suitable as the raw
// getWidth passed to WrapWithResizeDetection/LiveGetWidth, but not cheap
// enough to call on every clear() or every frame render directly. Errors
// once up front if file isn't a real terminal (or is nil); the returned func
// itself never errors, returning 0 instead if a later query fails (e.g. the
// file closed).
func WidthFromFile(file *os.File) (func() int, error) {
	if file == nil {
		return nil, errors.New("unable to determine width for nil file")
	}

	fd := int(file.Fd())
	_, _, err := term.GetSize(fd)
	if err != nil {
		return nil, err
	}
	return func() int {
		width, _, err := term.GetSize(fd)
		if err != nil {
			return 0
		} else {
			return width
		}
	}, nil
}

// SigwinchFromAny adapts a caller-owned <-chan any into spinq's plain
// "something changed" shape, ignoring the payload entirely - useful for
// folding some other event source (that isn't os.Signal) into a resize
// signal. A full destination buffer drops the extra value rather than
// blocking; coalescing duplicate signals is harmless since spinq only ever
// cares whether something changed, not how many times. The returned channel
// closes once in is closed.
func SigwinchFromAny(in <-chan any) <-chan struct{} {
	sigwinch := make(chan struct{}, 1)
	go func() {
		defer close(sigwinch)
		for range in {
			select {
			case sigwinch <- struct{}{}:
			default:
			}
		}
	}()
	return sigwinch
}

// SigwinchFromPoller returns a <-chan struct{} that signals once every d,
// for platforms (Windows) with no real resize event to hook into. d is
// entirely your choice - it's the ceiling on how stale the terminal width
// can get before spinq notices, traded directly against how often the real
// getWidth ends up getting called. ctx governs its lifetime: cancelling it
// stops the background goroutine and closes the returned channel. Like
// DefaultSigwinch and unlike Every, that cleanup is not optional - an
// abandoned goroutine here is a live GC root that never gets collected.
func SigwinchFromPoller(ctx context.Context, d time.Duration) <-chan struct{} {
	sigwinch := make(chan struct{}, 1)
	go func() {
		defer close(sigwinch)
		ticker := time.NewTicker(d)
		for {
			select {
			case <-ticker.C:
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
