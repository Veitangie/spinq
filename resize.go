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
// behind a cheap, shareable one: it calls the real getWidth once up front
// and again on each sigwinch signal, caching the result in an atomic the
// returned func just reads. Only one reader can safely drain a given
// sigwinch channel, but the returned func itself can be called freely
// from anywhere - always prefer passing its output into
// Dynamic/DynamicRender/WrapWithResizeDetection over a raw getWidth.
func CachedGetWidth(sigwinch <-chan struct{}, getWidth func() int) func() int {
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
// real terminal (or is nil); the returned func itself never errors,
// returning 0 if a later query fails (e.g. the file closed).
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
// getWidth ends up getting called. A non-positive d returns nil (a channel
// that never signals). ctx governs its lifetime: cancelling it stops the
// background goroutine and closes the returned channel; a nil ctx returns
// an already-closed channel instead, without starting a goroutine. Like
// DefaultSigwinch and unlike Every, that cleanup is not optional - an
// abandoned goroutine here is a live GC root that never gets collected.
func SigwinchFromPoller(ctx context.Context, d time.Duration) <-chan struct{} {
	if d <= 0 {
		return nil
	}
	sigwinch := make(chan struct{}, 1)
	if ctx == nil {
		close(sigwinch)
		return sigwinch
	}
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

// DefaultResizeDetection is WithDefaultResizeDetection, but also returns
// the getWidth it wired up so it can be reused elsewhere - e.g. shaped with
// Offset/Portion/Clamp to size a DynamicBarRender/Dynamic FrameFunc built
// for the same JustStart call, or fetched later via SpinqWriter.GetWidth.
// On failure (usually os.Stderr not being a real terminal), both return
// values stay safe to use unconditionally: the JustStartOptionsFunc falls
// back to the same no-op WithDefaultResizeDetection's own failure path
// uses, and getWidth reports -1 (nothing to detect), the same value
// GetWidth() reports for any writer with no resize detection to draw
// against. The error is still returned so the caller can tell success from
// failure if they care to.
func DefaultResizeDetection(ctx context.Context) (JustStartOptionsFunc, func() int, error) {
	getWidth, err := DefaultGetWidth(ctx)
	if err != nil {
		return noop(), func() int { return -1 }, err
	}

	return WithResizeDetection(getWidth), getWidth, nil
}

// WrapDefaultResizeDetection is WrapWithDefaultResizeDetection, but also
// returns the getWidth it wired up, for the same reasons and with the same
// safe-on-failure behavior as DefaultResizeDetection - see its doc comment.
func WrapDefaultResizeDetection(ctx context.Context) (WrapOptionsFunc, func() int, error) {
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
