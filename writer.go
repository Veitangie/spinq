// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
	"context"
	"fmt"
	"io"
)

// Writer is a normal io.WriteCloser, plus the spinner's own lifecycle
// control and error reporting. A slow FrameFunc inside Start/StopNoClear/a
// running Set/Close blocks that call, any other Start/Stop*/Set/Close made
// concurrently on the same Writer, and Close itself. Write is unaffected.
//
//   - Start begins drawing immediately and on every tick of the Pair's
//     ticker, until Stop/StopWith/StopNoClear/Close, or ctx is cancelled.
//     A nil ctx defaults to the Pair's own governing context. A call
//     while already running is a no-op returning nil. Fetches and draws
//     the first frame synchronously before returning.
//   - Stop halts the spinner and clears its last-drawn frame.
//   - StopWith halts the spinner and replaces its frame with a message,
//     without clearing.
//   - StopNoClear fetches one final frame synchronously before freezing
//     the display, optionally followed by a raw suffix; a failed fetch
//     leaves the previous frame untouched instead of blanking it.
//   - Set installs a new FrameFunc. While running, it also fetches and
//     draws from it synchronously before returning. While stopped, it
//     only stores the FrameFunc - nothing is drawn until the next Start.
//   - Close stops the spinner, clears its display, and waits for the
//     actor to fully exit, including any FrameFunc call still in flight.
//     Never closes the underlying wrapped io.Writer, even if it
//     implements io.Closer. Start/Stop/StopWith/StopNoClear/Set all
//     return ErrClosed once called after Close. Close itself is
//     idempotent - a second call is a no-op returning nil, same as the
//     first. Doesn't guarantee every goroutine spinq ever spawned has
//     exited yet - a couple of short-lived internal ones are cancelled,
//     not joined.
//   - Err returns a channel of errors not otherwise reportable
//     synchronously: a write failure during a ticker redraw or Close's
//     final clear, or a PanicError when a FrameFunc call panics. Best-effort
//     delivery; closes once the writer is fully shut down (including after
//     Close - ranging over it is safe unconditionally). Only a write
//     failure auto-stops the spinner - a PanicError just skips that frame.
//   - IsReal reports whether this writer is backed by a live spinner
//     actor (true) or is a no-op passthrough (false) - true forever, even
//     after Close.
//   - GetWidth returns the getWidth func this writer uses to size its own
//     rendering. Never nil: -1 means resize detection isn't configured -
//     safe to call GetWidth()() with no nil check, before or after Close.
//
// Two implementations exist: an unexported one backed by a live spinner
// actor, and WriterPassthrough, used whenever WrapFilePair/WrapOS/
// JustStart detect a non-terminal stream - every lifecycle method above
// is a no-op on it, and Err returns an already-closed channel.
type Writer interface {
	io.WriteCloser
	Start(context.Context) error
	Stop() error
	StopWith(string) error
	StopNoClear(string) error
	Set(FrameFunc) error
	IsReal() bool
	GetWidth() WidthFunc
	Err() <-chan error
}

// WriterPassthrough is a Writer that writes straight through to the
// wrapped io.Writer. Start/Stop/StopWith/StopNoClear/Set/Close are all
// no-ops, and Err returns an already-closed channel. WrapFilePair,
// WrapOS, and JustStart fall back to this automatically for any stream
// that isn't a real terminal, so a spinner never corrupts redirected or
// piped output.
type WriterPassthrough struct {
	io.Writer
}

var _ Writer = WriterPassthrough{}

func (sw WriterPassthrough) Start(_ context.Context) error { return nil }

func (sw WriterPassthrough) Stop() error { return nil }

func (sw WriterPassthrough) StopWith(_ string) error { return nil }

func (sw WriterPassthrough) StopNoClear(_ string) error { return nil }

func (sw WriterPassthrough) Set(_ FrameFunc) error { return nil }

func (sw WriterPassthrough) IsReal() bool { return false }

func (sw WriterPassthrough) GetWidth() WidthFunc { return func() int { return -1 } }

func (sw WriterPassthrough) Close() error { return nil }

func (sw WriterPassthrough) Err() <-chan error {
	res := make(chan error)
	close(res)
	return res
}

type writerReal struct {
	st       *spinnerState
	wrapped  io.Writer
	getWidth WidthFunc
	errCh    <-chan error
}

var _ Writer = writerReal{}

func (sw writerReal) Write(data []byte) (int, error) {
	sw.st.writerMut.Lock()
	defer sw.st.writerMut.Unlock()

	if clearErr := sw.st.clear(); clearErr != nil {
		sw.st.frame = []byte{}
		var msg any = reportError{fmt.Errorf("failed to clear writer: %w", clearErr)}
		go fireEvent(msg, sw.st.task)
	}
	sw.st.canWrite = false

	written, err := sw.wrapped.Write(data)

	if written > 0 && written <= len(data) {
		sw.st.canWrite = data[written-1] == '\n'
	}

	if drawErr := sw.st.draw(); drawErr != nil {
		sw.st.frame = []byte{}
		var msg any = reportError{fmt.Errorf("failed to draw spinner back: %w", drawErr)}
		go fireEvent(msg, sw.st.task)
	}
	return written, err
}

func (sw writerReal) Start(ctx context.Context) error {
	return sw.st.start(ctx)
}

func (sw writerReal) Stop() error {
	return sw.st.stop()
}

func (sw writerReal) StopWith(message string) error {
	return sw.st.stopWith(message)
}

func (sw writerReal) StopNoClear(message string) error {
	return sw.st.stopNoClear(message)
}

func (sw writerReal) Set(getFrame FrameFunc) error {
	return sw.st.setGetFrame(getFrame)
}

func (sw writerReal) IsReal() bool { return true }

func (sw writerReal) GetWidth() WidthFunc { return sw.getWidth }

func (sw writerReal) Close() error {
	sw.st.close()
	<-sw.st.closed
	return nil
}

func (sw writerReal) Err() <-chan error {
	return sw.errCh
}

type stdWriter struct {
	underlying writerReal
}

var _ io.Writer = stdWriter{writerReal{}}

func (std stdWriter) Write(data []byte) (int, error) {
	return std.underlying.Write(data)
}
