// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

package spinq

import (
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
					if st.running {
						typed.notify <- ErrAlreadyRunning
						close(typed.notify)
						continue
					}

					frame, err := st.safeGetFrame(st.getFrame, st.revision)
					st.running = true
					st.notifyStopped = typed.notifyStopped

					if err == nil {
						err = st.set(frame)
						st.running = err == nil
					} else {
						st.writerMut.Lock()
						err = st.draw()
						st.writerMut.Unlock()
					}
					typed.notify <- err
					close(typed.notify)

				case stop:
					if !st.running {
						close(typed.notify)
						continue
					}

					st.running = false
					close(st.notifyStopped)
					st.notifyStopped = nil
					if !typed.clear {
						if lastFrame, err := st.safeGetFrame(st.getFrame, st.revision); err == nil {
							err = st.set(lastFrame)
							if err != nil {
								typed.notify <- err
								continue
							}
						}
					}

					var err error
					st.writerMut.Lock()
					if typed.clear {
						err = st.clear()
					}

					if err != nil {
						st.writerMut.Unlock()
						typed.notify <- err
						close(typed.notify)
						continue
					}

					if len(typed.lastFrame) != 0 {
						_, err = st.wrapped.Write(typed.lastFrame)
					}
					st.writerMut.Unlock()
					typed.notify <- err
					close(typed.notify)

				case drawFrame:
					if !st.running || typed.revision != st.revision {
						continue
					}

					err := st.set(typed.frame)
					if err != nil {
						st.running = false
						close(st.notifyStopped)
						st.notifyStopped = nil
						timer := time.NewTimer(10 * time.Millisecond)

						select {
						case st.errCh <- fmt.Errorf("failed to draw frame, stopping: %w", err):
						case <-timer.C:
						}
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

					if st.running {
						frame, err := st.safeGetFrame(st.getFrame, st.revision)
						if err == nil {
							err = st.set(frame)
							st.running = err == nil
							if err != nil {
								close(st.notifyStopped)
								st.notifyStopped = nil
							}
							typed.notify <- err
						}
					}

					close(typed.notify)
				default:
					continue
				}
			case <-st.ticker:
				if !st.running {
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

func (st *spinnerState) windDown() {
	st.running = false
	if st.notifyStopped != nil {
		close(st.notifyStopped)
		st.notifyStopped = nil
	}
	st.writerMut.Lock()
	err := st.clear()
	st.frame = []byte{}
	st.writerMut.Unlock()
	if err != nil {
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case st.errCh <- fmt.Errorf("failed to clear before closing: %w", err):
		case <-timer.C:
		}
	}
	close(st.errCh)
}
