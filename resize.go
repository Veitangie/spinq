// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

// WidthFunc reports the current terminal width on demand - see
// CachedGetWidth, WidthFromFile, and Offset/Portion/Clamp for building one.
type WidthFunc func() int

func zeroOnPanic(underlying WidthFunc) WidthFunc {
	return func() (res int) {
		defer func() {
			if recover() != nil {
				res = 0
			}
		}()

		res = underlying()
		return
	}
}

// CachedGetWidth wraps a (possibly expensive, e.g. syscall-backed) getWidth
// behind a cheap, shareable one: it calls the real getWidth once up front
// and again on each sigwinch signal, caching the result in an atomic the
// returned func just reads. Only one reader can safely drain a given
// sigwinch channel, but the returned func itself can be called freely
// from anywhere - always prefer passing its output into
// Dynamic/DynamicRender/WrapWithResizeDetection over a raw getWidth.
//
// The returned func never crashes on a panicking getWidth, reporting 0
// instead for that call - same fallback as a failed WidthFromFile query.
//
// CachedGetWidth spawns one background goroutine to watch sigwinch; it
// runs until sigwinch is closed. DefaultSigwinch and SigwinchFromPoller
// both close their channel when their ctx is cancelled - a hand-rolled
// sigwinch that never closes leaks the goroutine.
func CachedGetWidth(sigwinch <-chan struct{}, getWidth WidthFunc) WidthFunc {
	getWidth = zeroOnPanic(getWidth)
	current := atomic.Int64{}
	current.Store(int64(getWidth()))
	go func() {
		for range sigwinch {
		Drain:
			for {
				select {
				case _, ok := <-sigwinch:
					if !ok {
						break Drain
					}
				default:
					break Drain
				}
			}
			current.Store(int64(getWidth()))
		}
	}()
	return func() int {
		return int(current.Load())
	}
}

// WidthFromFile returns a getWidth func that queries file's terminal width
// on every call via a real syscall (x/term.GetSize) - suitable as the raw
// getWidth for CachedGetWidth, but too expensive to pass directly to
// WrapWithResizeDetection/WithResizeDetection or Dynamic/DynamicRender;
// wrap it in CachedGetWidth first. Errors once up front if file isn't a
// real terminal (or is nil), pairing that error with a func that always
// reports -1 ("nothing to detect"). On success the func never errors,
// reporting 0 for any later query that fails (e.g. the file closed).
func WidthFromFile(file *os.File) (WidthFunc, error) {
	if file == nil {
		return func() int { return -1 }, errors.New("unable to determine width for nil file")
	}

	fd := int(file.Fd())
	_, _, err := term.GetSize(fd)
	if err != nil {
		return func() int { return -1 }, err
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
// "something changed" shape, ignoring the payload entirely - for folding
// some other event source (that isn't os.Signal) into a resize signal. A
// full destination buffer drops the extra value rather than blocking;
// coalescing is harmless since spinq only cares whether something changed,
// not how many times. The returned channel closes once in is closed.
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
// for platforms (Windows) with no real resize event to hook into - the
// ceiling on how stale the terminal width can get before spinq notices,
// traded against how often the real getWidth gets called. ctx governs its
// lifetime: cancelling it stops the background goroutine and closes the
// returned channel; like DefaultSigwinch and unlike Every, that cleanup is
// not optional - an abandoned goroutine here is a live GC root that never
// gets collected. A non-positive d or nil context returns an
// already-closed channel instead, without starting a goroutine.
func SigwinchFromPoller(ctx context.Context, d time.Duration) <-chan struct{} {
	sigwinch := make(chan struct{}, 1)
	if ctx == nil || d <= 0 {
		close(sigwinch)
		return sigwinch
	}
	go func() {
		defer close(sigwinch)
		ticker := time.NewTicker(d)
		defer ticker.Stop()
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

// DefaultResizeDetection is WithDefaultResizeDetection, but also returns
// the getWidth it wired up, for reuse elsewhere - e.g. shaped with
// Offset/Portion/Clamp to size a DynamicBarRender/Dynamic FrameFunc for the
// same JustStart call, or fetched later via Writer.GetWidth.
//
// On failure (usually os.Stderr not being a real terminal), both return
// values stay safe to use unconditionally: the JustStartOptionsFunc is a
// no-op, and getWidth always reports -1 (the same "nothing to detect" value
// GetWidth() reports). The error is still returned so the caller can tell
// success from failure if they care to.
func DefaultResizeDetection(ctx context.Context) (JustStartOptionsFunc, WidthFunc, error) {
	getWidth, err := DefaultGetWidth(ctx)
	if err != nil {
		return noop(), func() int { return -1 }, err
	}

	return WithResizeDetection(getWidth), getWidth, nil
}

// WrapDefaultResizeDetection is WrapWithDefaultResizeDetection, but also
// returns the getWidth it wired up, for the same reasons and with the same
// safe-on-failure behavior as DefaultResizeDetection - see its doc comment.
func WrapDefaultResizeDetection(ctx context.Context) (WrapOptionsFunc, WidthFunc, error) {
	getWidth, err := DefaultGetWidth(ctx)
	if err != nil {
		return func(wo WrapOptions) WrapOptions { return wo }, func() int { return -1 }, err
	}

	return WrapWithResizeDetection(getWidth), getWidth, nil
}

// Offset returns width adjusted by delta columns - negative to reserve room
// for fixed-width content around a dynamic bar (e.g. a label or a percent
// counter), positive to pad. Composes with Portion/Clamp and with any other
// getWidth by nesting - e.g. Clamp(Offset(getWidth, -6), 10, 200).
func Offset(width WidthFunc, delta int) WidthFunc {
	return func() int { return width() + delta }
}

// Portion returns width scaled by portion, clamped to [0, 1] - 0.5 means
// half of whatever width reports. Pass its output anywhere a getWidth is
// expected, e.g. DynamicBarRender/DynamicSmoothBarRender/Dynamic itself.
func Portion(width WidthFunc, portion float64) WidthFunc {
	portion = max(0, min(1, portion))
	return func() int { return int(math.Floor(float64(width()) * portion)) }
}

// Clamp bounds width's result to [from, to] - useful to guarantee a
// readable minimum (or a sane maximum) regardless of how width was built,
// e.g. after Offset/Portion. from > to isn't a valid range; Clamp doesn't
// reject it, it just always returns from.
func Clamp(width WidthFunc, from, to int) WidthFunc {
	return func() int { return max(from, min(to, width())) }
}

func crop(width int, data []byte) []byte {
	iter := graphemeOpts.BytesGraphemes(data)
	total := 0
	canTakeMore := true
	result := bytes.Buffer{}
	for iter.Next() {
		size := iter.Width()
		cur := iter.Value()

		if size == 0 {
			result.Write(cur)
			continue
		}
		if !canTakeMore {
			continue
		}

		if total+size <= width {
			total += size
			result.Write(cur)
			continue
		}
		canTakeMore = false
	}

	return result.Bytes()
}
