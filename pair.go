// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
)

// Pair holds a program's two output surfaces around one spinner: Standard -
// a plain io.Writer for normal output, which the spinner clears out of the
// way and redraws around - and Spinner - the Writer for the animated frame
// itself, and the sole handle for lifecycle control and error reporting
// (Start/Stop/SetFrame/Close/Err). Both may wrap the same underlying
// stream; which concrete types they get depends on the constructor - see
// WrapPair, WrapFilePair, WrapOS, and JustStart.
type Pair struct {
	// Standard is typed as a plain io.Writer on purpose: the spinner is
	// driven only through Spinner. Writes to Standard are still
	// coordinated with the spinner so the two streams never corrupt each
	// other's output.
	Standard io.Writer
	Spinner  Writer
}

func passthroughPair(main, spinner io.Writer) *Pair {
	return &Pair{
		Standard: stdWriter{main},
		Spinner:  WriterPassthrough{spinner},
	}
}

// WrapOptions configures WrapPair/WrapFilePair/WrapOS; see WrapOptionsFunc
// and WrapWithResizeDetection. Its zero value (DefaultWrapOptions) leaves
// resize detection off.
type WrapOptions struct {
	GetWidth WidthFunc
}

// WrapOptionsFunc configures a WrapOptions value; see WrapWithResizeDetection.
type WrapOptionsFunc func(WrapOptions) WrapOptions

// DefaultWrapOptions returns WrapOptions' zero-config values: resize
// detection off.
func DefaultWrapOptions() WrapOptions {
	return WrapOptions{}
}

// WrapWithResizeDetection enables width-aware clearing/cropping for
// WrapPair/WrapFilePair/WrapOS: getWidth reports the current terminal width
// on demand, and is called directly with no caching of its own on every
// clear()/Write() - pass an already-cheap getWidth (typically
// CachedGetWidth's output), shaped with Offset/Portion/Clamp as needed.
// See WrapWithDefaultResizeDetection/WrapDefaultResizeDetection for
// zero-configuration sources. A nil getWidth is a no-op, leaving resize
// detection off. A panicking getWidth never crashes the process, reporting
// 0 for that call instead.
//
// You usually don't want this. A self-sizing frame (Dynamic,
// DynamicBarRender, DynamicSmoothBarRender) fits on its own and keeps the
// safe single-row clear; with resize detection on, the shrink cleanup can
// erase on-screen output above the spinner during a resize, on any
// terminal. Read the README's "On resizing" section before reaching for
// it.
func WrapWithResizeDetection(getWidth WidthFunc) WrapOptionsFunc {
	if getWidth == nil {
		return func(wo WrapOptions) WrapOptions { return wo }
	}
	return func(wo WrapOptions) WrapOptions {
		wo.GetWidth = getWidth
		return wo
	}
}

// WrapPair wraps main and spinner in a *Pair backed by a live spinner
// actor - this is the primitive every other Wrap*/JustStart entry point
// builds on. ctx governs the whole Pair's lifetime: cancelling
// it (or calling Close) stops the actor and makes every subsequent call
// return ErrClosed; a nil ctx defaults to context.Background(). getFrame
// supplies frames on demand and is called by the actor on its own
// schedule (start, ticker, and SetFrame), never concurrently with itself.
// ticker drives periodic redraws; see Every for a simple wall-clock
// source, or supply your own channel (e.g. for tests). A nil or closed
// ticker means no periodic redraws - drive them via Write/SetFrame, or
// install a ticker later with Writer.SetTicker.
//
// If main or spinner is nil, the other is used for both. It is an error for
// both to be nil or for getFrame to be nil.
func WrapPair(ctx context.Context, main, spinner io.Writer, getFrame FrameFunc, ticker <-chan time.Time, opts ...WrapOptionsFunc) (*Pair, error) {
	if main == nil && spinner == nil {
		return nil, errors.New("both writers are nil")
	}
	if getFrame == nil {
		return nil, errors.New("frame function is nil")
	}

	if main == nil {
		main = spinner
	}
	if spinner == nil {
		spinner = main
	}
	if ctx == nil {
		ctx = context.Background()
	}

	opt := DefaultWrapOptions()
	for _, f := range opts {
		if f != nil {
			opt = f(opt)
		}
	}

	var cd clearerDrawer = obliviousClearerDrawer{}

	getWidth := func() int { return -1 }
	if opt.GetWidth != nil {
		getWidth = zeroOnPanic(opt.GetWidth)
		cd = &awareClearerDrawer{
			getWidth: getWidth,
			width:    getWidth(),
		}
	}

	withCancel, cancel := context.WithCancel(ctx)
	closed := make(chan struct{})
	errCh := make(chan error, 1)
	st := &spinnerState{
		writerMut: &sync.Mutex{},
		wrapped:   spinner,
		wg:        &sync.WaitGroup{},
		cd:        cd,
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

	return &Pair{
		Standard: stdWriter{
			writerReal{
				st:       st,
				wrapped:  main,
				getWidth: getWidth,
				errCh:    errCh,
			},
		},
		Spinner: writerReal{
			st:       st,
			wrapped:  spinner,
			getWidth: getWidth,
			errCh:    errCh,
		},
	}, nil
}

// WrapFilePair is WrapPair for *os.File streams: it checks whether spinner
// (and, if that's a terminal, main too) is actually a terminal via isatty,
// falling back to a plain passthrough - no actor, no mutex, with
// Start/Stop/SetFrame/SetTicker all no-ops - for either stream that isn't.
// Safe to call unconditionally on redirected output (a pipe, a file, CI
// logs). Both streams are wrapped via go-colorable for correct ANSI
// rendering on legacy Windows terminals. WrapOS is this function applied
// to os.Stdout/os.Stderr.
func WrapFilePair(ctx context.Context, main, spinner *os.File, getFrame FrameFunc, ticker <-chan time.Time, opts ...WrapOptionsFunc) (*Pair, error) {
	if main == nil && spinner == nil {
		return nil, errors.New("both files are nil")
	}
	if main == nil {
		main = spinner
	}
	if spinner == nil {
		spinner = main
	}

	if getFrame == nil {
		return nil, errors.New("frame function is nil")
	}

	colorableMain, colorableSpinner := colorable.NewColorable(main), colorable.NewColorable(spinner)

	inTermSpinner := isatty.IsTerminal(spinner.Fd()) || isatty.IsCygwinTerminal(spinner.Fd())
	if !inTermSpinner {
		return passthroughPair(colorableMain, colorableSpinner), nil
	}

	inTermStandard := isatty.IsTerminal(main.Fd()) || isatty.IsCygwinTerminal(main.Fd())
	res, err := WrapPair(ctx, colorableMain, colorableSpinner, getFrame, ticker, opts...)
	if err != nil {
		return nil, err
	}

	if !inTermStandard {
		res.Standard = stdWriter{colorableMain}
	}
	return res, nil
}

// WrapOS wraps os.Stdout/os.Stderr via WrapFilePair, so the spinner is
// automatically disabled (falling back to a plain passthrough) whenever
// either stream isn't a real terminal. It also disables itself outright
// under a CI environment variable, without even touching os.Stdout/Stderr,
// so it's safe to call from any CI runner regardless of how that runner's
// own TTY detection behaves. This is the entry point JustStart itself uses.
func WrapOS(ctx context.Context, getFrame FrameFunc, ticker <-chan time.Time, opts ...WrapOptionsFunc) (*Pair, error) {
	if getFrame == nil {
		return nil, errors.New("frame function is nil")
	}

	if _, ok := os.LookupEnv("CI"); ok {
		return passthroughPair(os.Stdout, os.Stderr), nil
	}

	return WrapFilePair(ctx, os.Stdout, os.Stderr, getFrame, ticker, opts...)
}
