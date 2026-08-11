// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
	"context"
	"io"
)

// SpinqWriter is the interface shared by SpinqPair's Standard and Spinny
// writers: a normal io.Writer, plus the spinner's own lifecycle control.
//
//   - Start begins drawing frames immediately and on every tick of the
//     Pair's ticker, until Stop/StopWith/StopNoClear is called, ctx is
//     cancelled, or the Pair is Closed. Calling it while already running is
//     a no-op that returns nil.
//   - Stop halts the spinner and clears its last-drawn frame.
//   - StopWith halts the spinner and replaces its frame with a message,
//     without clearing.
//   - StopNoClear forces one final getFrame fetch (so a progress-style
//     spinner reports its truest final reading) before freezing the
//     display, optionally followed by a raw suffix; if that final fetch
//     fails, the previous frame is left untouched rather than blanked.
//   - Set installs a new FrameFunc, taking effect on the very next redraw.
//
// Two implementations exist: SpinqWriterReal, backed by a live spinner
// actor, and SpinqWriterPassthrough, a plain passthrough used whenever
// WrapFilePair/WrapOS/JustStart detect a non-terminal stream - on which
// every lifecycle method above is a no-op.
type SpinqWriter interface {
	io.Writer
	Start(context.Context) error
	Stop() error
	StopWith(string) error
	StopNoClear(string) error
	Set(FrameFunc) error
	close()
}

// SpinqWriterPassthrough is a SpinqWriter that writes straight through to
// the wrapped io.Writer, with every lifecycle method (Start, Stop,
// StopWith, StopNoClear, Set) a no-op. WrapFilePair, WrapOS, and JustStart
// fall back to this automatically for any stream that isn't a real
// terminal, so a spinner never corrupts redirected or piped output.
type SpinqWriterPassthrough struct {
	io.Writer
}

var _ SpinqWriter = SpinqWriterPassthrough{}

func (sw SpinqWriterPassthrough) Start(_ context.Context) error { return nil }

func (sw SpinqWriterPassthrough) Stop() error { return nil }

func (sw SpinqWriterPassthrough) StopWith(_ string) error { return nil }

func (sw SpinqWriterPassthrough) StopNoClear(_ string) error { return nil }

func (sw SpinqWriterPassthrough) Set(_ FrameFunc) error { return nil }

func (sw SpinqWriterPassthrough) close() {}

// SpinqWriterReal is a SpinqWriter backed by a live spinner actor, as
// constructed by WrapPair.
type SpinqWriterReal struct {
	st      *spinnerState
	wrapped io.Writer
}

var _ SpinqWriter = SpinqWriterReal{}

// Write clears the spinner, writes data to the wrapped stream, then redraws
// the spinner - so a spinner animating on the same stream as regular output
// never gets its frame interleaved with or corrupted by that output. The
// spinner is only redrawn if data ends in a newline, so a redraw doesn't
// happen mid-line.
func (sw SpinqWriterReal) Write(data []byte) (int, error) {
	sw.st.writerMut.Lock()
	defer sw.st.writerMut.Unlock()

	err := sw.st.clear()
	sw.st.canWrite = false
	if err != nil {
		return 0, err
	}

	written, err := sw.wrapped.Write(data)
	if err != nil {
		return written, err
	}

	if written > 0 && written <= len(data) {
		sw.st.canWrite = data[written-1] == '\n'
	}

	return written, sw.st.draw()
}

func (sw SpinqWriterReal) Start(ctx context.Context) error {
	return sw.st.start(ctx)
}

func (sw SpinqWriterReal) Stop() error {
	return sw.st.stop()
}

func (sw SpinqWriterReal) StopWith(message string) error {
	return sw.st.stopWith(message)
}

func (sw SpinqWriterReal) StopNoClear(message string) error {
	return sw.st.stopNoClear(message)
}

func (sw SpinqWriterReal) Set(getFrame FrameFunc) error {
	return sw.st.setGetFrame(getFrame)
}

func (sw SpinqWriterReal) close() {
	sw.st.close()
	<-sw.st.closed
}
