// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// SpinqPair bundles two SpinqWriters sharing one spinner: Standard for the
// program's normal output (which the spinner clears out of the way and
// redraws around), and Spinny for the animated frame itself, plus lifecycle
// control (Start/Stop/Set) shared by both. Standard and Spinny may be the
// same underlying stream, or fall back to one another, depending on how the
// pair was constructed - see WrapPair, WrapFilePair, WrapOS, and JustStart.
type SpinqPair struct {
	Standard SpinqWriter
	Spinny   SpinqWriter
	err      <-chan error
}

// Close stops the spinner, clears its display, and shuts down the
// background actor, waiting for it to fully exit before returning. Every
// spinner lifecycle management method on Standard/Spinny called after Close
// returns ErrClosed, but Write does not get affected.
func (sp SpinqPair) Close() {
	sp.Spinny.close()
}

// Err returns a channel of errors from failures spinq can't otherwise
// report synchronously - specifically, a write failure during a
// ticker-triggered redraw, or during the final clear on Close. Deliveries
// are best-effort: if nothing is reading from the channel when an error
// occurs, spinq waits briefly before giving up and moving on, rather than
// blocking on a reader that may never come. The channel is closed once the
// Pair is fully shut down.
func (sp SpinqPair) Err() <-chan error {
	return sp.err
}

func passthroughPair(main, spinny io.Writer) *SpinqPair {
	errCh := make(chan error)
	close(errCh)
	return &SpinqPair{
		Standard: SpinqWriterPassthrough{main},
		Spinny:   SpinqWriterPassthrough{spinny},
		err:      errCh,
	}
}

// WrapPair wraps main and spinny in a *SpinqPair backed by a real,
// running spinner actor - this is the primitive every other Wrap*/JustStart
// entry point builds on. ctx governs the whole Pair's lifetime: cancelling
// it (or calling Close) stops the actor and makes every subsequent call
// return ErrClosed. getFrame supplies frames on demand and is called by the
// actor on its own schedule (start, ticker, and Set), never concurrently
// with itself. ticker drives periodic redraws; see Every for a simple
// wall-clock source, or supply your own channel (e.g. for tests).
//
// If main or spinny is nil, the other is used for both. It is an error for
// both to be nil, for getFrame to be nil, or for ticker to be nil.
func WrapPair(ctx context.Context, main, spinny io.Writer, getFrame FrameFunc, ticker <-chan time.Time) (*SpinqPair, error) {
	if main == nil && spinny == nil {
		return nil, errors.New("both writers are nil")
	}

	if getFrame == nil {
		return nil, errors.New("frame function is nil")
	}

	if ticker == nil {
		return nil, errors.New("ticker for spinner is nil")
	}

	if main == nil {
		main = spinny
	}
	if spinny == nil {
		spinny = main
	}

	withCancel, cancel := context.WithCancel(ctx)
	closed := make(chan struct{})
	errCh := make(chan error, 1)
	st := &spinnerState{
		writerMut: &sync.Mutex{},
		wrapped:   spinny,
		sg:        &singleflight.Group{},
		wg:        &sync.WaitGroup{},
		errCh:     errCh,
		task:      make(chan any),
		ticker:    ticker,
		getFrame:  getFrame,
		running:   &atomic.Bool{},
		canWrite:  true,
		ctx:       withCancel,
		close:     cancel,
		closed:    closed,
	}
	st.startBackground()

	return &SpinqPair{
		Standard: SpinqWriterReal{
			st:      st,
			wrapped: main,
		},
		Spinny: SpinqWriterReal{
			st:      st,
			wrapped: spinny,
		},
		err: errCh,
	}, nil
}

// WrapFilePair is WrapPair for *os.File streams, with an added safety net:
// it stats spinny (and, if that's a terminal, main too) and falls back to a
// plain passthrough - no actor, no mutex, Start/Stop/Set become no-ops -
// for either stream that isn't actually a character device (a real
// terminal). This is what makes it safe to call unconditionally on
// redirected output (a pipe, a file, CI logs) without a spinner corrupting
// non-interactive output. WrapOS is this function applied to
// os.Stdout/os.Stderr.
func WrapFilePair(ctx context.Context, main, spinny *os.File, getFrame FrameFunc, ticker <-chan time.Time) (*SpinqPair, error) {
	if getFrame == nil {
		return nil, errors.New("frame function is nil")
	}

	if ticker == nil {
		return nil, errors.New("ticker for spinner is nil")
	}

	fi, err := spinny.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat spinny: %w", err)
	}
	inTermErr := fi.Mode()&os.ModeCharDevice != 0
	if !inTermErr {
		return passthroughPair(main, spinny), nil
	}

	fi, err = main.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat main: %w", err)
	}
	inTermOut := fi.Mode()&os.ModeCharDevice != 0

	res, err := WrapPair(ctx, main, spinny, getFrame, ticker)
	if err != nil {
		return nil, err
	}

	if !inTermOut {
		res.Standard = SpinqWriterPassthrough{main}
	}
	return res, nil
}

// WrapOS wraps os.Stdout/os.Stderr via WrapFilePair, so the spinner is
// automatically disabled (falling back to a plain passthrough) whenever
// either stream isn't a real terminal. It also disables itself outright
// under a CI environment variable, without even touching os.Stdout/Stderr,
// so it's safe to call from any CI runner regardless of how that runner's
// own TTY detection behaves. This is the entry point JustStart itself uses.
func WrapOS(ctx context.Context, getFrame FrameFunc, ticker <-chan time.Time) (*SpinqPair, error) {
	if getFrame == nil {
		return nil, errors.New("frame function is nil")
	}

	if ticker == nil {
		return nil, errors.New("ticker for spinner is nil")
	}

	if _, ok := os.LookupEnv("CI"); ok {
		return passthroughPair(os.Stdout, os.Stderr), nil
	}

	return WrapFilePair(ctx, os.Stdout, os.Stderr, getFrame, ticker)
}
