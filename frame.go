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

// ErrNoFrame signals that a FrameFunc has no frame to offer on this call.
// It is not a failure: Join treats an ErrNoFrame segment as "keep showing
// whatever it last rendered" rather than blanking it out, so returning
// ErrNoFrame is the correct way for a FrameFunc to say "nothing new yet."
var ErrNoFrame error = errors.New("no frame")

// Preset state sequences for use with Simple, SimpleOnceEvery, Random, and
// RandomOnceEvery. These are plain []string values, copied into a FrameFunc
// closure's own private state at construction time, so mutating one of
// these package-level slices afterward has no effect on FrameFuncs already
// built from it.
var (
	DotsStates      []string = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	LineStates      []string = []string{"-", "\\", "|", "/"}
	ArrowStates     []string = []string{"↑", "↗", "→", "↘", "↓", "↙", "←", "↖"}
	PipeStates      []string = []string{"┤", "┘", "┴", "└", "├", "┌", "┬", "┐"}
	FlyTroughStates []string = []string{"[    ]", "[=   ]", "[==  ]", "[ == ]", "[  ==]", "[   =]", "[    ]"}
	BounceStates    []string = []string{"[    ]", "[=   ]", "[==  ]", "[ == ]", "[  ==]", "[   =]", "[    ]", "[   =]", "[  ==]", "[ == ]", "[==  ]", "[=   ]"}
	GrowingStates   []string = []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█", "▇", "▆", "▅", "▄", "▃", "▂", "▁"}
	BinaryStates    []string = []string{"010010", "001100", "100101", "111010", "011000", "111100", "110101", "100010"}
)

// FrameFunc produces one frame of spinner/progress output on each call.
// spinq's actor guarantees a given FrameFunc is never called concurrently
// with itself, so a FrameFunc's own private state (like Simple's internal
// index) never needs its own locking. If a FrameFunc reads external state
// that other goroutines also write, synchronizing that access is the
// FrameFunc's (and its caller's) responsibility, not spinq's.
//
// Returning a non-nil error signals "no frame this call" rather than a
// failure - see ErrNoFrame and Join.
type FrameFunc func() ([]byte, error)

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

// SimpleOnceEvery returns a FrameFunc like Simple, except it only advances
// to the next state once every mod calls instead of on every call - useful
// for slowing a spinner's cycle down relative to how often it's drawn. A
// mod of 1 behaves exactly like Simple. An empty states or a non-positive
// mod returns Noop; a single state returns Static.
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
	Format  func(time.Duration) string
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
func DefaultDurationFormat() func(d time.Duration) string {
	return func(d time.Duration) string {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
}

// DurationOptionsFunc configures a DurationOptions value; see
// DurationWithFormat and DurationStartAt.
type DurationOptionsFunc func(DurationOptions) DurationOptions

// DurationWithFormat sets Duration's format function, which renders the
// elapsed time.Duration as the frame's text.
func DurationWithFormat(format func(d time.Duration) string) DurationOptionsFunc {
	return func(do DurationOptions) DurationOptions {
		do.Format = format
		return do
	}
}

// DurationStartAt sets an explicit start time for Duration, overriding its
// default of capturing the time of the FrameFunc's first call.
func DurationStartAt(startAt time.Time) DurationOptionsFunc {
	return func(do DurationOptions) DurationOptions {
		do.StartAt = &startAt
		return do
	}
}

// Duration returns a FrameFunc that renders the elapsed time since a start
// point, formatted by opt.Format (DefaultDurationFormat by default). Unless
// DurationStartAt overrides it, the start point is captured lazily on the
// FrameFunc's first call rather than when Duration itself is constructed,
// so any gap between building the frame and actually starting the spinner
// doesn't inflate the first reading. A nil timer returns Noop.
func Duration(timer func() time.Time, opts ...DurationOptionsFunc) FrameFunc {
	if timer == nil {
		return Noop()
	}
	opt := DefaultDurationOptions()
	for _, f := range opts {
		opt = f(opt)
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
// every call. By default it draws from math/rand/v2's package-level
// (concurrency-safe) source; passing a *rand.Rand uses that instead - in
// which case, since spinq guarantees this FrameFunc is never called
// concurrently with itself, the *rand.Rand only needs its own
// synchronization if something outside this FrameFunc also touches it.
// Extra *rand.Rand arguments beyond the first are ignored. An empty states
// returns Noop; a single state returns Static.
func Random(states []string, rands ...*rand.Rand) FrameFunc {
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

	return func() ([]byte, error) {
		idx := 0
		if len(rands) != 0 {
			idx = rands[0].IntN(len(statesBytes))
		} else {
			idx = rand.IntN(len(statesBytes))
		}
		chosen := statesBytes[idx]
		return chosen, nil
	}
}

// RandomOnceEvery returns a FrameFunc like Random, except it only draws a
// new state once every mod calls instead of on every call, showing that
// draw for exactly mod calls before drawing again. A mod of 1 behaves
// exactly like Random. An empty states or a non-positive mod returns Noop;
// a single state returns Static.
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

	statesBytes := make([][]byte, 0, len(states))
	for _, state := range states {
		statesBytes = append(statesBytes, []byte(state))
	}

	skipper := 0
	idx := 0
	if len(rands) != 0 {
		idx = rands[0].IntN(len(statesBytes))
	} else {
		idx = rand.IntN(len(statesBytes))
	}
	return func() ([]byte, error) {
		if skipper == mod {
			skipper = 0
			if len(rands) != 0 {
				idx = rands[0].IntN(len(statesBytes))
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
// (current, total) reading. It is the one place that validates the
// reading: RenderFuncs are contractually allowed to assume total > 0 and
// current <= total, so Progress returns ErrNoFrame instead of calling
// render whenever that doesn't hold - leaving Join free to keep showing
// the last good segment rather than a broken one.
func Progress(progress func() (int, int), render RenderFunc) FrameFunc {
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
// call fetches a fresh frame from every non-nil FrameFunc, but a segment
// that returns an error keeps rendering its last successful frame instead
// of going blank - so one segment having a transient hiccup doesn't blank
// out the whole joined line. If every segment errors on a given call (none
// has ever produced a frame yet), Join itself returns ErrNoFrame.
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

// WidthFunc builds a fresh FrameFunc for a given terminal width - see
// Dynamic. BarRender/SmoothBarRender's length parameter is usually what a
// WidthFunc closes over to produce a correctly-sized render pipeline.
type WidthFunc func(width int) FrameFunc

// Dynamic returns a FrameFunc that rebuilds itself via build whenever
// getWidth's value changes, so the resulting content (e.g. a progress bar)
// tracks the terminal's current width instead of a size fixed at
// construction. build is only called again on an actual change - between
// changes, Dynamic just keeps calling the FrameFunc build last returned, so
// animated/progressing content (a moving spinner, an advancing percentage)
// keeps updating every call, not just on resize.
//
// getWidth is called on every call to the returned FrameFunc - once per
// frame render, which spinq treats as a hot path. Always pass the func
// CachedGetWidth returns here, never a raw syscall-backed getWidth directly:
// CachedGetWidth is what makes this cheap, both for a single Dynamic instance
// and for however many are joined together in one frame, since they can all
// share its output rather than each paying for their own real query.
func Dynamic(getWidth func() int, build WidthFunc) FrameFunc {
	width := getWidth()
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
