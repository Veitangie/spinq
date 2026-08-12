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
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

var clearBytes []byte = []byte("\r\033[K")

// ErrClosed is returned by any SpinqWriter method called after the Pair has
// been Closed (or its governing context cancelled).
var ErrClosed error = errors.New("spinner closed")

// ErrAlreadyRunning is an internal signal used between the background actor
// and Start's caller-side code to distinguish a genuine fresh start from a
// redundant call while already running. It is never actually returned to a
// SpinqWriter.Start caller - Start translates it to a nil (successful,
// idempotent) result.
var ErrAlreadyRunning error = errors.New("spinner already running")

type spinnerState struct {
	wrapped io.Writer
	ticker  <-chan time.Time
	sg      *singleflight.Group
	wg      *sync.WaitGroup
	errCh   chan<- error

	ctx           context.Context
	close         context.CancelFunc
	closed        chan struct{}
	notifyStopped chan struct{}

	task     chan any
	getFrame FrameFunc
	running  *atomic.Bool
	revision uint64

	writerMut *sync.Mutex
	needClear bool
	canWrite  bool
	frame     []byte
}

// NOT THREAD SAFE
func (st *spinnerState) clear() error {
	if st.needClear {
		st.needClear = false
		_, err := st.wrapped.Write(clearBytes)
		return err
	}
	return nil
}

// NOT THREAD SAFE
func (st *spinnerState) draw() error {
	if st.running.Load() && st.canWrite && !st.needClear && len(st.frame) != 0 {
		st.needClear = true
		_, err := st.wrapped.Write(st.frame)
		return err
	}
	return nil
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
	st.revision += 1
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

func (st *spinnerState) safeGetFrame(getFrame FrameFunc, revision uint64) ([]byte, error) {
	select {
	case res := <-st.sg.DoChan(strconv.FormatUint(revision, 16), recoverOnPanic(getFrame)):
		if res.Err != nil {
			return []byte{}, res.Err
		}
		if typed, ok := res.Val.([]byte); ok {
			return typed, nil
		}
		return []byte{}, ErrNoFrame

	case <-st.ctx.Done():
		return nil, ErrClosed
	}
}

func recoverOnPanic(underlying func() ([]byte, error)) func() (any, error) {
	return func() (res any, err error) {
		defer func() {
			maybePanic := recover()
			if maybePanic != nil {
				res = []byte{}
				err = fmt.Errorf("%v", maybePanic)
			}
		}()

		res, err = underlying()
		return
	}
}
