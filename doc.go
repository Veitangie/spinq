// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

// Package spinq is a small, focused, actor-based terminal spinner and
// progress-bar library - no TUI framework underneath, just the spinner,
// the bar, and resize-aware rendering.
//
// # Getting started
//
// JustStart is the zero-config entry point: it wraps os.Stdout/os.Stderr,
// falls back to a silent passthrough when either isn't a real terminal,
// and calls Start for you.
//
//	pair, err := spinq.JustStart(spinq.WithText("Working"))
//	if err != nil { panic(err) }
//	defer pair.Spinner.Close()
//	fmt.Fprintln(pair.Standard, "normal program output")
//
// pair.Standard is a plain io.Writer for the program's own output;
// pair.Spinner is the Writer that renders the frame and carries the
// lifecycle methods (Start, Stop, StopWith, StopNoClear, SetFrame,
// SetTicker, Close, Err). Writes to Standard are coordinated with the
// spinner so the two streams never corrupt each other, even on the same
// terminal.
//
// WrapOS, WrapFilePair, and WrapPair are the lower layers, each exported
// if you want fewer defaults; WrapPair is the primitive and takes any two
// io.Writers.
//
// # Frames
//
// A frame is a FrameFunc (func() ([]byte, error)), produced by composing
// small primitives - Simple/Random over a state set, Duration, Progress
// with a RenderFunc (BarRender, SmoothBarRender, PercentRender,
// FractRender, JoinRender), and Join/Surrounded/Static to assemble them.
// spinq never calls a FrameFunc concurrently with itself. A non-nil error
// from a FrameFunc means "nothing new this call", not a failure; a panic
// is recovered and reported on Err as a PanicError.
//
// # Resizing
//
// spinq never queries the terminal size on its own. For a width-responsive
// bar, size it with a Dynamic frame over a cached WidthFunc (see
// DefaultGetWidth, CachedGetWidth, Offset/Portion/Clamp) and leave
// WithResizeDetection off - see the "On resizing" section of the README
// for why.
package spinq
