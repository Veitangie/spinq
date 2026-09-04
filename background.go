// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
	"bytes"
	"errors"
	"fmt"
	"time"
)

type start struct {
	notify        chan error
	notifyStopped chan struct{}
}

type stop struct {
	lastFrame []byte
	notify    chan error
	clear     bool
}

type setGetFrame struct {
	getFrame FrameFunc
	notify   chan error
}

type setTicker struct {
	ticker <-chan time.Time
	notify chan struct{}
}

type drawFrame struct {
	revision uint64
	frame    []byte
	err      error
}

type reportError struct {
	err error
}

func fireEvent[T any](t T, chT chan<- T) {
	defer func() {
		_ = recover()
	}()
	select {
	case chT <- t:
	case <-time.After(10 * time.Millisecond):
	}
}

func (st *spinnerState) startBackground() {
	go func() {
		defer close(st.closed)
		for {
			select {
			case task, ok := <-st.task:
				if !ok {
					// Impossible case, but still better to just safely exit
					st.windDown()
					return
				}

				switch typed := task.(type) {
				case start:
					if st.running.Load() {
						typed.notify <- errAlreadyRunning
						close(typed.notify)
						continue
					}

					st.wg.Wait()
					frame, err := st.safeGetFrame(st.getFrame)
					st.running.Store(true)
					st.notifyStopped = typed.notifyStopped

					if err == nil {
						err = st.set(frame)
						if err != nil {
							st.stopFromActor()
							typed.notify <- err
						}
					}
					close(typed.notify)

				case stop:
					if !st.running.Load() {
						close(typed.notify)
						continue
					}

					st.stopFromActor()
					lastFrame := typed.lastFrame
					prefix := []byte{}
					if !typed.clear {
						st.wg.Wait()

						if maybePrefix, err := st.safeGetFrame(st.getFrame); err == nil {
							prefix = maybePrefix
						}
					}

					var err error
					st.writerMut.Lock()
					if len(prefix) != 0 && !bytes.Equal(prefix, st.frame) {
						lastFrame = append(prefix, lastFrame...)
						typed.clear = true
					}

					if typed.clear {
						err = st.clear()
					}
					st.frame = []byte{}

					if len(lastFrame) != 0 {
						_, errLastFrame := st.wrapped.Write(lastFrame)
						if err != nil && errLastFrame != nil {
							err = errors.Join(err, errLastFrame)
						} else if errLastFrame != nil {
							err = errLastFrame
						}
					}
					st.writerMut.Unlock()
					typed.notify <- err
					close(typed.notify)

				case drawFrame:
					st.inFlight = false
					if typed.revision != st.revision || !st.running.Load() {
						continue
					}

					if typed.err != nil {
						continue
					}

					err := st.set(typed.frame)
					if err != nil {
						st.stopFromActor()
						fireEvent(fmt.Errorf("failed to draw frame, stopping: %w", err), st.errCh)
					}

				case setGetFrame:
					if typed.getFrame == nil {
						typed.notify <- errors.New("unable to set nil FrameFunc")
						close(typed.notify)
						continue
					}

					st.getFrame = typed.getFrame
					st.revision += 1

					st.wg.Wait()

					if st.running.Load() {
						frame, err := st.safeGetFrame(st.getFrame)
						if err == nil {
							err = st.set(frame)
							if err != nil {
								st.stopFromActor()
							}
							typed.notify <- err
						}
					}

					close(typed.notify)

				case setTicker:
					st.ticker = typed.ticker
					close(typed.notify)

				case reportError:
					st.stopFromActor()
					fireEvent(typed.err, st.errCh)

				default:
					continue
				}
			case _, ok := <-st.ticker:
				if !ok {
					st.ticker = nil
					continue
				}

				if st.inFlight || !st.running.Load() {
					continue
				}

				st.wg.Add(1)
				st.inFlight = true
				go func(getFrame FrameFunc, revision uint64) {
					frame, err := getFrame()
					st.wg.Done()
					select {
					case st.task <- drawFrame{
						revision: revision,
						frame:    frame,
						err:      err,
					}:
					case <-st.ctx.Done():
					}
				}(st.safeGetFrameFunc(st.getFrame), st.revision)

			case <-st.ctx.Done():
				st.windDown()
				return
			}
		}
	}()
}

// NOT THREAD SAFE
func (st *spinnerState) windDown() {
	st.running.Store(false)
	if st.notifyStopped != nil {
		close(st.notifyStopped)
		st.notifyStopped = nil
	}
	st.wg.Wait()
	st.writerMut.Lock()
	err := st.clear()
	st.writerMut.Unlock()
	if err != nil {
		fireEvent(err, st.errCh)
	}
	close(st.errCh)
}

// NOT THREAD SAFE
func (st *spinnerState) stopFromActor() {
	st.running.Store(false)
	if st.notifyStopped != nil {
		close(st.notifyStopped)
		st.notifyStopped = nil
	}
}
