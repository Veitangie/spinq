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

type drawFrame struct {
	revision uint64
	frame    []byte
}

type reportError struct {
	err error
}

func fireEvent[T any](t T, chT chan<- T) {
	timer := time.NewTimer(10 * time.Millisecond)
	defer func() {
		_ = recover()
		timer.Stop()
	}()
	select {
	case chT <- t:
	case <-timer.C:
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
						typed.notify <- ErrAlreadyRunning
						close(typed.notify)
						continue
					}

					frame, err := st.safeGetFrame(st.getFrame, st.revision)
					st.running.Store(true)
					st.notifyStopped = typed.notifyStopped

					if err == nil {
						err = st.set(frame)
						if err != nil {
							st.stopFromBg()
							typed.notify <- err
						}
					}
					close(typed.notify)

				case stop:
					if !st.running.Load() {
						close(typed.notify)
						continue
					}

					st.stopFromBg()
					lastFrame := typed.lastFrame
					prefix := []byte{}
					if !typed.clear {
						st.revision += 1
						st.wg.Wait()

						if maybePrefix, err := st.safeGetFrame(st.getFrame, st.revision); err == nil {
							prefix = maybePrefix
						}
					}

					var err error
					st.writerMut.Lock()
					if len(prefix) != 0 && !bytes.Equal(prefix, st.frame) {
						lastFrame = append(prefix, lastFrame...)
						typed.clear = true
					}

					if typed.clear && st.needClear {
						lastFrame = append(clearBytes, lastFrame...)
						st.needClear = false
					}
					st.frame = []byte{}

					if len(lastFrame) != 0 {
						_, err = st.wrapped.Write(lastFrame)
					}
					st.writerMut.Unlock()
					typed.notify <- err
					close(typed.notify)

				case drawFrame:
					if !st.running.Load() || typed.revision != st.revision {
						continue
					}

					err := st.set(typed.frame)
					if err != nil {
						st.stopFromBg()
						fireEvent(fmt.Errorf("failed to draw frame, stopping: %w", err), st.errCh)
					}

				case setGetFrame:
					if typed.getFrame == nil {
						typed.notify <- errors.New("unable to set nil FrameFunc")
						close(typed.notify)
						continue
					}

					st.getFrame = typed.getFrame
					// Invalidate all stale in-flight getFrame calls
					st.revision += 1

					st.wg.Wait()

					if st.running.Load() {
						frame, err := st.safeGetFrame(st.getFrame, st.revision)
						if err == nil {
							err = st.set(frame)
							if err != nil {
								st.stopFromBg()
							}
							typed.notify <- err
						}
					}

					close(typed.notify)
				case reportError:
					st.stopFromBg()
					fireEvent(typed.err, st.errCh)
				default:
					continue
				}
			case <-st.ticker:
				if !st.running.Load() {
					continue
				}

				st.wg.Add(1)
				go func(getFrame FrameFunc, revision uint64) {
					frame, err := st.safeGetFrame(getFrame, revision)
					st.wg.Done()
					if err == nil {
						select {
						case st.task <- drawFrame{
							revision: revision,
							frame:    frame,
						}:
						case <-st.ctx.Done():
						}
					}
				}(st.getFrame, st.revision)

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
	st.writerMut.Lock()
	err := st.clear()
	st.writerMut.Unlock()
	if err != nil {
		fireEvent(err, st.errCh)
	}
	close(st.errCh)
}

// NOT THREAD SAFE
func (st *spinnerState) stopFromBg() {
	st.running.Store(false)
	if st.notifyStopped != nil {
		close(st.notifyStopped)
		st.notifyStopped = nil
	}
}
