// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
	"context"
	"errors"
	"math"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

// CachedGetWidth wraps a (possibly expensive, e.g. syscall-backed) getWidth
// behind a cheap, freely shareable one: it calls the real getWidth once up
// front and again each time sigwinch signals, caching the result in an
// atomic that the returned func just reads. This is the one legitimate
// consumer of a raw sigwinch channel for width purposes - channels aren't
// broadcast, so only one reader can safely drain a given sigwinch, but many
// readers can safely call the func CachedGetWidth hands back.
//
// This is the piece that makes width-aware code cheap to call from a hot
// path: awareClearerDrawer's clear() (driven by WrapWithResizeDetection) and
// any number of Dynamic-built FrameFuncs can all share the *same*
// CachedGetWidth output and each just do an atomic read, rather than each
// hitting the real getWidth independently. Always prefer passing
// CachedGetWidth's output into Dynamic over a raw getWidth - Dynamic is called
// on every frame render, and a raw syscall-backed getWidth there reintroduces
// exactly the per-call cost this function exists to avoid.
func CachedGetWidth(sigwinch <-chan struct{}, getWidth func() int) func() int {
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
// getWidth passed into CachedGetWidth, but not cheap enough to call on every
// clear() or every frame render directly, so never hand its output straight
// to WrapWithResizeDetection/WithResizeDetection or Dynamic/DynamicRender -
// wrap it yourself or pass it to CachedGetWidth first. Errors once up front if file isn't a real
// terminal (or is nil); the returned func itself never errors, returning 0
// instead if a later query fails (e.g. the file closed).
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

// Offset returns width adjusted by delta columns - negative to reserve room
// for fixed-width content around a dynamic bar (e.g. a label or a percent
// counter), positive to pad. Composes with Portion/Clamp and with any other
// getWidth by nesting - e.g. Clamp(Offset(getWidth, -6), 10, 200).
func Offset(width func() int, delta int) func() int {
	return func() int { return width() + delta }
}

// Portion returns width scaled by portion, clamped to [0, 1] - 0.5 means
// half of whatever width reports. Pass its output anywhere a getWidth is
// expected, e.g. DynamicBarRender/DynamicSmoothBarRender/Dynamic itself.
func Portion(width func() int, portion float64) func() int {
	portion = max(0, min(1, portion))
	return func() int { return int(math.Floor(float64(width()) * portion)) }
}

// Clamp bounds width's result to [from, to] - useful to guarantee a
// readable minimum (or a sane maximum) regardless of how width was built,
// e.g. after Offset/Portion.
func Clamp(width func() int, from, to int) func() int {
	return func() int { return max(from, min(to, width())) }
}
