// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

// ErrNoFrame signals a FrameFunc has nothing new to offer this call - not a
// failure. Join keeps showing a segment's last frame instead of blanking it
// out when it returns this.
var ErrNoFrame error = errors.New("no frame")

// Preset state sequences for Simple, SimpleOnceEvery, Random, and
// RandomOnceEvery. Copied into each FrameFunc's own state at construction,
// so mutating one of these slices afterward doesn't affect FrameFuncs
// already built from it.
var (
	DotsStates       []string = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	LineStates       []string = []string{"-", "\\", "|", "/"}
	ArrowStates      []string = []string{"↑", "↗", "→", "↘", "↓", "↙", "←", "↖"}
	PipeStates       []string = []string{"┤", "┘", "┴", "└", "├", "┌", "┬", "┐"}
	FlyThroughStates []string = []string{"[    ]", "[=   ]", "[==  ]", "[ == ]", "[  ==]", "[   =]", "[    ]"}
	BounceStates     []string = []string{"[    ]", "[=   ]", "[==  ]", "[ == ]", "[  ==]", "[   =]", "[    ]", "[   =]", "[  ==]", "[ == ]", "[==  ]", "[=   ]"}
	GrowingStates    []string = []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█", "▇", "▆", "▅", "▄", "▃", "▂", "▁"}
	BinaryStates     []string = []string{"010010", "001100", "100101", "111010", "011000", "111100", "110101", "100010"}
)

// FrameFunc produces one frame of spinner/progress output on each call.
// spinq never calls a given FrameFunc concurrently with itself, so its own
// private state (like Simple's internal index) never needs locking.
// External state a FrameFunc reads that other goroutines also write is the
// caller's own responsibility to synchronize.
//
// spinq keeps the returned slice - it compares it against the next call's
// output and may redraw it later - so a FrameFunc must not mutate bytes it
// has already returned; copy out of any reused buffer first.
//
// A non-nil error means "no frame this call", not a failure - see
// ErrNoFrame and Join.
type FrameFunc func() ([]byte, error)

// DurationFormatFunc renders an elapsed time.Duration as a frame's text -
// see Duration and DurationWithFormat.
type DurationFormatFunc func(time.Duration) string

// ProgressFunc reports a (current, total) reading - see Progress, which
// enforces total > 0 and current <= total before ever calling render.
type ProgressFunc func() (int, int)

// Noop returns a FrameFunc that always renders an empty frame.
func Noop() FrameFunc {
	return func() ([]byte, error) { return []byte{}, nil }
}

// Static returns a FrameFunc that always renders state, unchanged.
func Static(state string) FrameFunc {
	byteState := []byte(state)
	return func() ([]byte, error) {
		return byteState, nil
	}
}

// Simple returns a FrameFunc that cycles through states in order, advancing
// to the next state on every call and wrapping back to the first after the
// last. An empty states returns Noop; a single state returns Static.
func Simple(states []string) FrameFunc {
	if len(states) == 0 {
		return Noop()
	}
	if len(states) == 1 {
		return Static(states[0])
	}

	statesBytes := make([][]byte, 0, len(states))
	for _, state := range states {
		statesBytes = append(statesBytes, []byte(state))
	}
	idx := 0

	return func() ([]byte, error) {
		chosen := statesBytes[idx]
		idx = (idx + 1) % len(statesBytes)
		return chosen, nil
	}
}

// SimpleOnceEvery is Simple, but only advances to the next state once
// every mod calls instead of on every call - slows the cycle down relative
// to how often it's drawn. mod == 1 behaves exactly like Simple. An empty
// states or non-positive mod returns Noop; a single state returns Static.
func SimpleOnceEvery(states []string, mod int) FrameFunc {
	if len(states) == 0 || mod <= 0 {
		return Noop()
	}
	if len(states) == 1 {
		return Static(states[0])
	}
	if mod == 1 {
		return Simple(states)
	}

	statesBytes := make([][]byte, 0, len(states))
	for _, state := range states {
		statesBytes = append(statesBytes, []byte(state))
	}
	idx := 0
	skipper := 0

	return func() ([]byte, error) {
		if skipper == mod {
			skipper = 0
			idx = (idx + 1) % len(statesBytes)
		}
		chosen := statesBytes[idx]
		skipper += 1
		return chosen, nil
	}
}

// DurationOptions configures Duration. StartAt, if nil, defaults to the
// moment of the FrameFunc's first call rather than when Duration itself was
// constructed - see Duration.
type DurationOptions struct {
	StartAt *time.Time
	Format  DurationFormatFunc
}

// DefaultDurationOptions returns the default DurationOptions: an unset
// StartAt (captured lazily on first call) and DefaultDurationFormat.
func DefaultDurationOptions() DurationOptions {
	return DurationOptions{
		Format: DefaultDurationFormat(),
	}
}

// DefaultDurationFormat returns the default duration formatter, which
// renders a duration as seconds with one decimal place (e.g. "2.3s").
func DefaultDurationFormat() DurationFormatFunc {
	return func(d time.Duration) string {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
}

// DurationOptionsFunc configures a DurationOptions value; see
// DurationWithFormat and DurationWithStartAt.
type DurationOptionsFunc func(DurationOptions) DurationOptions

// DurationWithFormat sets Duration's format function, which renders the
// elapsed time.Duration as the frame's text. A nil format is a no-op,
// leaving any previously configured Format untouched.
func DurationWithFormat(format DurationFormatFunc) DurationOptionsFunc {
	if format == nil {
		return func(do DurationOptions) DurationOptions { return do }
	}

	return func(do DurationOptions) DurationOptions {
		do.Format = format
		return do
	}
}

// DurationWithStartAt sets an explicit start time for Duration, overriding its
// default of capturing the time of the FrameFunc's first call.
func DurationWithStartAt(startAt time.Time) DurationOptionsFunc {
	return func(do DurationOptions) DurationOptions {
		do.StartAt = &startAt
		return do
	}
}

// Duration returns a FrameFunc that renders the elapsed time since a start
// point, formatted by opt.Format (DefaultDurationFormat by default). Unless
// DurationWithStartAt overrides it, the start point is captured lazily on
// the FrameFunc's first call, not when Duration is constructed. A nil
// timer, or an opt.Format left (or set) nil, returns Noop.
func Duration(timer func() time.Time, opts ...DurationOptionsFunc) FrameFunc {
	if timer == nil {
		return Noop()
	}
	opt := DefaultDurationOptions()
	for _, f := range opts {
		if f != nil {
			opt = f(opt)
		}
	}

	if opt.Format == nil {
		return Noop()
	}

	var startAt time.Time
	if opt.StartAt != nil {
		startAt = *opt.StartAt
	}
	captured := opt.StartAt != nil

	return func() ([]byte, error) {
		now := timer()
		if !captured {
			captured = true
			startAt = now
		}
		if now.Before(startAt) {
			return []byte{}, nil
		}
		return []byte(opt.Format(now.Sub(startAt))), nil
	}
}

// Random returns a FrameFunc that picks a state uniformly at random on
// every call, drawing from math/rand/v2's package-level (concurrency-safe)
// source by default. The first non-nil entry in rands is used instead if
// there is one - only needs its own synchronization if something outside
// this FrameFunc also touches it, since spinq itself never calls it
// concurrently. Remaining rands entries are ignored. An empty states
// returns Noop; a single state returns Static.
func Random(states []string, rands ...*rand.Rand) FrameFunc {
	if len(states) == 0 {
		return Noop()
	}
	if len(states) == 1 {
		return Static(states[0])
	}

	var actualRand *rand.Rand
	for _, rnd := range rands {
		if rnd != nil {
			actualRand = rnd
			break
		}
	}

	statesBytes := make([][]byte, 0, len(states))
	for _, state := range states {
		statesBytes = append(statesBytes, []byte(state))
	}

	return func() ([]byte, error) {
		idx := 0
		if actualRand != nil {
			idx = actualRand.IntN(len(statesBytes))
		} else {
			idx = rand.IntN(len(statesBytes))
		}
		chosen := statesBytes[idx]
		return chosen, nil
	}
}

// RandomOnceEvery is Random, but only draws a new state once every mod
// calls instead of on every call, showing that draw for exactly mod calls
// before drawing again. mod == 1 behaves exactly like Random. An empty
// states or non-positive mod returns Noop; a single state returns Static.
func RandomOnceEvery(states []string, mod int, rands ...*rand.Rand) FrameFunc {
	if len(states) == 0 || mod <= 0 {
		return Noop()
	}
	if len(states) == 1 {
		return Static(states[0])
	}
	if mod == 1 {
		return Random(states, rands...)
	}

	var actualRand *rand.Rand
	for _, rnd := range rands {
		if rnd != nil {
			actualRand = rnd
			break
		}
	}

	statesBytes := make([][]byte, 0, len(states))
	for _, state := range states {
		statesBytes = append(statesBytes, []byte(state))
	}

	skipper := 0
	idx := 0
	if actualRand != nil {
		idx = actualRand.IntN(len(statesBytes))
	} else {
		idx = rand.IntN(len(statesBytes))
	}
	return func() ([]byte, error) {
		if skipper == mod {
			skipper = 0
			if actualRand != nil {
				idx = actualRand.IntN(len(statesBytes))
			} else {
				idx = rand.IntN(len(statesBytes))
			}
		}
		skipper += 1
		return statesBytes[idx], nil
	}
}

// Surrounded returns a FrameFunc that wraps delegate's output between
// prefix and suffix. It propagates delegate's error unchanged and, on
// error, renders nothing (not even prefix/suffix). A nil delegate returns
// Noop.
func Surrounded(prefix string, delegate FrameFunc, suffix string) FrameFunc {
	if delegate == nil {
		return Noop()
	}

	prefixBytes := []byte(prefix)
	suffixBytes := []byte(suffix)

	return func() ([]byte, error) {
		wrapped, err := delegate()
		if err != nil {
			return []byte{}, err
		}
		res := make([]byte, 0, len(prefix)+len(wrapped)+len(suffix))
		res = append(res, prefixBytes...)
		res = append(res, wrapped...)
		res = append(res, suffixBytes...)
		return res, nil
	}
}

// Progress returns a FrameFunc that renders a progress bar/counter from a
// (current, total) reading. The one place that validates the reading -
// RenderFuncs may assume total > 0 and current <= total - returning
// ErrNoFrame instead of calling render whenever that doesn't hold. A nil
// progress or nil render returns Noop.
func Progress(progress ProgressFunc, render RenderFunc) FrameFunc {
	if progress == nil || render == nil {
		return Noop()
	}
	return func() ([]byte, error) {
		current, total := progress()
		if current > total || total <= 0 {
			return []byte{}, ErrNoFrame
		}
		return render(current, total), nil
	}
}

// Join returns a FrameFunc that renders each of fs joined by sep. Nil
// entries in fs are dropped; if every entry is nil, Join returns Noop. Each
// call fetches a fresh frame from every non-nil FrameFunc; a segment that
// errors keeps rendering its last successful frame instead of going blank.
// If every segment has errored on every call so far, Join returns
// ErrNoFrame.
func Join(sep string, fs ...FrameFunc) FrameFunc {
	nils := 0
	for _, f := range fs {
		if f == nil {
			nils += 1
		}
	}

	var fsNoNils []FrameFunc
	if nils == 0 {
		fsNoNils = fs
	} else {
		fsNoNils = make([]FrameFunc, 0, len(fs)-nils)
		for _, f := range fs {
			if f != nil {
				fsNoNils = append(fsNoNils, f)
			}
		}
	}

	if len(fsNoNils) == 0 {
		return Noop()
	}

	cache := make([][]byte, len(fsNoNils))
	for i := range cache {
		cache[i] = []byte{}
	}
	sepBytes := []byte(sep)

	return func() ([]byte, error) {
		atLeastOneNew := false
		for i, f := range fsNoNils {
			res, err := f()
			if err == nil {
				cache[i] = res
				atLeastOneNew = true
			}
		}

		if !atLeastOneNew {
			return []byte{}, ErrNoFrame
		}
		return bytes.Join(cache, sepBytes), nil
	}
}

// WidthFrameFunc builds a fresh FrameFunc for a given terminal width - see
// Dynamic. BarRender/SmoothBarRender's length parameter is usually what a
// WidthFrameFunc closes over to produce a correctly-sized render pipeline.
type WidthFrameFunc func(width int) FrameFunc

// CropToWidth adapts a plain FrameFunc into a WidthFrameFunc that crops the
// source frame to the width it is handed - grapheme-aware, keeping
// zero-width sequences (so a trailing "\033[0m" still lands) while dropping
// the tail past the limit. Its point is Dynamic's build argument:
// Dynamic(getWidth, CropToWidth(frame)) makes any frame self-size, which
// keeps the safe single-row clear instead of needing WithResizeDetection
// (see the README's "On resizing" section).
//
// A width <= 0 leaves only the zero-width content - an effectively empty
// frame, the same degradation DynamicBarRender has. A nil source yields
// Noop; a source error passes straight through.
func CropToWidth(source FrameFunc) WidthFrameFunc {
	if source == nil {
		return func(width int) FrameFunc {
			return Noop()
		}
	}

	return func(width int) FrameFunc {
		return func() ([]byte, error) {
			frame, err := source()
			if err != nil {
				return []byte{}, err
			}
			return crop(width, frame), nil
		}
	}
}

// Dynamic returns a FrameFunc that rebuilds itself via build whenever
// getWidth's value changes; between changes, it just keeps calling the
// FrameFunc build last returned, so animated content keeps updating every
// call, not just on resize.
//
// getWidth is called on every call to the returned FrameFunc - a hot
// path. Always pass CachedGetWidth's output here, never a raw
// syscall-backed getWidth directly; see CachedGetWidth. A nil getWidth or
// nil build returns Noop.
func Dynamic(getWidth WidthFunc, build WidthFrameFunc) FrameFunc {
	if getWidth == nil || build == nil {
		return Noop()
	}
	width := zeroOnPanic(getWidth)()
	current := build(width)

	return func() ([]byte, error) {
		newWidth := getWidth()
		if newWidth != width {
			width = newWidth
			current = build(width)
		}
		return current()
	}
}
