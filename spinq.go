// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
	"context"
	"fmt"
	"time"
)

// Every returns a ticker channel that fires roughly every d - a thin
// convenience wrapper around time.NewTicker(d).C for use as WrapPair's or
// JustStart's ticker argument. A non-positive d returns nil (a channel
// that never fires) instead of panicking; WrapPair/WrapFilePair/WrapOS
// reject a nil ticker with a clean error.
func Every(d time.Duration) <-chan time.Time {
	if d <= 0 {
		return nil
	}
	return time.NewTicker(d).C
}

// JustStartOptions configures JustStart. See Default for its zero-config
// values.
//
// Frame, if set, is used verbatim and Text/States/Divider are ignored
// entirely. If Frame is left nil, JustStart composes a default template
// from Text/States/Divider instead - so those three fields are only ever
// consulted when Frame itself is unset, regardless of the order options
// were applied in.
type JustStartOptions struct {
	Context      context.Context
	StartContext context.Context
	Ticker       <-chan time.Time
	Frame        FrameFunc
	States       []string
	Text         string
	Divider      string
	GetWidth     func() int
}

// JustStartOptionsFunc configures a JustStartOptions value.
type JustStartOptionsFunc func(JustStartOptions) JustStartOptions

func noop() JustStartOptionsFunc {
	return func(jso JustStartOptions) JustStartOptions { return jso }
}

// WithJustStartOptions replaces the entire JustStartOptions with opt,
// discarding any options applied earlier in the same JustStart call.
func WithJustStartOptions(opt JustStartOptions) JustStartOptionsFunc {
	return func(jso JustStartOptions) JustStartOptions { return opt }
}

// WithContext sets the context governing the whole Pair's lifetime - the
// same ctx WrapOS/WrapPair receive. If StartContext is left unset, it
// defaults to this same context, so cancelling it also stops the spinner;
// see WithStartContext to give the two independent lifetimes. A nil ctx is
// a no-op, leaving any previously configured Context untouched.
func WithContext(ctx context.Context) JustStartOptionsFunc {
	if ctx == nil {
		return noop()
	}
	return func(jso JustStartOptions) JustStartOptions {
		jso.Context = ctx
		return jso
	}
}

// WithStartContext sets the context passed to the initial Start call,
// independent of the Context governing the Pair's own lifetime. Unlike
// WithContext, a nil ctx is not a no-op: it actively resets StartContext
// back to unset (meaning "inherit from Context"), even if a real value was
// configured by an earlier option in the same call.
func WithStartContext(ctx context.Context) JustStartOptionsFunc {
	return func(jso JustStartOptions) JustStartOptions {
		jso.StartContext = ctx
		return jso
	}
}

// WithTicker sets the ticker channel driving redraws. A nil ticker is a
// no-op, leaving any previously configured Ticker untouched.
func WithTicker(ticker <-chan time.Time) JustStartOptionsFunc {
	if ticker == nil {
		return noop()
	}

	return func(jso JustStartOptions) JustStartOptions {
		jso.Ticker = ticker
		return jso
	}
}

// WithDuration sets the ticker driving redraws to Every(d). A non-positive
// d (see Every) makes JustStart fail with a clean error.
func WithDuration(d time.Duration) JustStartOptionsFunc {
	return func(jso JustStartOptions) JustStartOptions {
		jso.Ticker = Every(d)
		return jso
	}
}

// WithFrame sets an explicit FrameFunc, bypassing JustStart's default
// template (and any WithText/WithStates/WithDivider options) entirely. A
// nil frame is a no-op, leaving any previously configured Frame untouched.
func WithFrame(frame FrameFunc) JustStartOptionsFunc {
	if frame == nil {
		return noop()
	}

	return func(jso JustStartOptions) JustStartOptions {
		jso.Frame = frame
		return jso
	}
}

// WithText sets the label used by JustStart's default frame template, e.g.
// "Uploading" for " ⠋ Uploading (2.3s)". Ignored if Frame is also set.
func WithText(text string) JustStartOptionsFunc {
	return func(jso JustStartOptions) JustStartOptions {
		jso.Text = text
		return jso
	}
}

// WithStates sets the spinner states used by JustStart's default frame
// template, in place of DotsStates. Ignored if Frame is also set.
func WithStates(states []string) JustStartOptionsFunc {
	return func(jso JustStartOptions) JustStartOptions {
		jso.States = states
		return jso
	}
}

// WithDivider sets the separator joining the segments of JustStart's
// default frame template (spinner, label+duration). Ignored if Frame is
// also set.
func WithDivider(div string) JustStartOptionsFunc {
	return func(jso JustStartOptions) JustStartOptions {
		jso.Divider = div
		return jso
	}
}

// WithResizeDetection enables width-aware clearing/cropping for JustStart:
// getWidth reports the current terminal width on demand, and may be
// called on every clear()/Write(), not just on a resize - pass an
// already-cheap source (typically CachedGetWidth's output), shaped with
// Offset/Portion/Clamp as needed. See WithDefaultResizeDetection/
// DefaultResizeDetection for zero-configuration sources. A nil getWidth
// is a no-op, leaving resize detection off.
func WithResizeDetection(getWidth func() int) JustStartOptionsFunc {
	if getWidth == nil {
		return noop()
	}
	return func(jso JustStartOptions) JustStartOptions {
		jso.GetWidth = getWidth
		return jso
	}
}

// Default returns JustStartOptions' zero-config values: a background
// Context, a Ticker firing every 100ms, and a Frame left unset so JustStart
// composes its default "⠋ Running (2.3s)"-style template from Text
// ("Running") and States (DotsStates).
func Default() JustStartOptions {
	return JustStartOptions{
		Context:      context.Background(),
		StartContext: nil,
		Ticker:       Every(100 * time.Millisecond),
		Frame:        nil,
		Text:         "Running",
		States:       DotsStates,
	}
}

// JustStart is the zero-configuration entry point for the common case: it
// wraps WrapOS with sensible defaults (see Default) and starts the spinner
// immediately, returning the resulting *SpinqPair. It does not defer
// Stop/Close - that remains the caller's responsibility, same as WrapOS.
func JustStart(opts ...JustStartOptionsFunc) (*SpinqPair, error) {
	opt := Default()
	for _, f := range opts {
		if f != nil {
			opt = f(opt)
		}
	}
	if opt.StartContext == nil {
		opt.StartContext = opt.Context
	}
	if opt.Frame == nil {
		opt.Frame = Join(opt.Divider, Surrounded(" ", Simple(opt.States), fmt.Sprintf(" %s (", opt.Text)), Duration(time.Now), Static(")"))
	}
	wrapOpts := make([]WrapOptionsFunc, 0, 1)
	if opt.GetWidth != nil {
		wrapOpts = append(wrapOpts, WrapWithResizeDetection(opt.GetWidth))
	}

	pair, err := WrapOS(opt.Context, opt.Frame, opt.Ticker, wrapOpts...)
	if err != nil {
		return nil, err
	}

	err = pair.Spinny.Start(opt.StartContext)
	return pair, err
}
