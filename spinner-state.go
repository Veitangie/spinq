// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// ErrClosed is returned by any Writer lifecycle method (via Pair.Spinner)
// called after Close (or its governing context cancellation).
var ErrClosed error = errors.New("spinner closed")

// ErrAlreadyRunning signals a redundant Start call while already running.
// Start never returns it to the caller - it's translated to nil.
var ErrAlreadyRunning error = errors.New("spinner already running")

// Panic is the error reported on Err() when a FrameFunc call panics - the
// actor recovers it, so a panic never crashes the process or propagates to
// a caller, and (unlike a write failure) never stops the spinner either;
// that one frame is just skipped. See Value for the original recovered
// value.
type Panic struct {
	underlying any
}

// Error renders the panic's recovered value via fmt.Sprint, satisfying the
// error interface.
func (p Panic) Error() string {
	return fmt.Sprint(p.underlying)
}

// Value returns the original value passed to panic(), unwrapped - useful
// when it's a specific error or type the caller wants to inspect rather
// than just render as a string via Error.
func (p Panic) Value() any {
	return p.underlying
}

type spinnerState struct {
	wrapped io.Writer
	ticker  <-chan time.Time
	wg      *sync.WaitGroup
	errCh   chan<- error
	cd      clearerDrawer

	ctx           context.Context
	close         context.CancelFunc
	closed        chan struct{}
	notifyStopped chan struct{}

	task     chan any
	getFrame FrameFunc
	running  *atomic.Bool
	revision uint64
	inFlight bool

	writerMut *sync.Mutex
	needClear bool
	canWrite  bool
	frame     []byte
}

// NOT THREAD SAFE
func (st *spinnerState) clear() error {
	return st.cd.clear(st)
}

// NOT THREAD SAFE
func (st *spinnerState) draw() error {
	return st.cd.draw(st)
}

// NOT THREAD SAFE
func (st *spinnerState) set(frame []byte) error {
	st.writerMut.Lock()
	defer st.writerMut.Unlock()
	if bytes.Equal(st.frame, frame) {
		return nil
	}

	err := st.clear()
	if err != nil {
		return err
	}

	st.frame = frame
	st.cd.adjust(st)
	err = st.draw()
	if err != nil {
		st.frame = []byte{}
	}
	return err
}

func (st *spinnerState) start(ctx context.Context) error {
	msg := start{notify: make(chan error, 1), notifyStopped: make(chan struct{})}
	select {
	case st.task <- msg:
	case <-st.ctx.Done():
		return ErrClosed
	}
	if ctx == nil {
		ctx = st.ctx
	}

	select {
	case err := <-msg.notify:
		if err == nil {
			err = nil
			go func() {
				select {
				case <-ctx.Done():
					st.stop() //nolint:staticcheck,errcheck
				case <-st.ctx.Done():
				case <-msg.notifyStopped:
				}
			}()
		}
		if errors.Is(err, ErrAlreadyRunning) {
			return nil
		}
		return err
	case <-st.ctx.Done():
		return ErrClosed
	}
}

func (st *spinnerState) stop() error {
	msg := stop{clear: true}
	return st.commonStop(msg)
}

func (st *spinnerState) stopWith(message string) error {
	msg := stop{clear: true, lastFrame: []byte(message)}
	return st.commonStop(msg)
}

func (st *spinnerState) stopNoClear(suffix string) error {
	msg := stop{lastFrame: []byte(suffix)}
	return st.commonStop(msg)
}

func (st *spinnerState) commonStop(msg stop) error {
	msg.notify = make(chan error, 1)
	select {
	case st.task <- msg:
	case <-st.ctx.Done():
		return ErrClosed
	}

	select {
	case err := <-msg.notify:
		return err
	case <-st.ctx.Done():
		return ErrClosed
	}
}

func (st *spinnerState) setGetFrame(getFrame FrameFunc) error {
	msg := setGetFrame{getFrame: getFrame, notify: make(chan error, 1)}
	select {
	case st.task <- msg:
	case <-st.ctx.Done():
		return ErrClosed
	}

	select {
	case err := <-msg.notify:
		return err
	case <-st.ctx.Done():
		return ErrClosed
	}
}

func (st *spinnerState) safeGetFrame(getFrame FrameFunc) (res []byte, err error) {
	defer func() {
		maybePanic := recover()
		if maybePanic != nil {
			fireEvent[error](Panic{maybePanic}, st.errCh)
			res = []byte{}
			err = ErrNoFrame
		}
	}()

	res, err = getFrame()
	return
}

func (st *spinnerState) safeGetFrameFunc(getFrame FrameFunc) FrameFunc {
	return func() ([]byte, error) { return st.safeGetFrame(getFrame) }
}
