// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0

// Command comparison renders the same fake pipeline two ways: "naive" hand-
// rolls a spinner on stderr with no coordination with the stdout writes
// happening alongside it, and "spinq" runs the identical pipeline through
// spinq instead. It exists to record the comparison GIF at the top of the
// README, not as a usage example.
package main

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"veitangie.dev/spinq"
)

type step struct {
	name string
	dur  time.Duration
}

var steps = []step{
	{"resolve config", 120 * time.Millisecond},
	{"connect db", 180 * time.Millisecond},
	{"run migrations", 220 * time.Millisecond},
	{"seed fixtures", 150 * time.Millisecond},
	{"auth: login", 140 * time.Millisecond},
	{"auth: refresh token", 130 * time.Millisecond},
	{"GET /users", 160 * time.Millisecond},
	{"POST /orders", 190 * time.Millisecond},
	{"poll shipment", 170 * time.Millisecond},
	{"verify webhook", 150 * time.Millisecond},
	{"check inventory", 200 * time.Millisecond},
	{"export report", 160 * time.Millisecond},
	{"rotate secrets", 140 * time.Millisecond},
	{"flush logs", 110 * time.Millisecond},
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: comparison naive|spinq")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "naive":
		runNaive()
	case "spinq":
		runSpinq()
	default:
		fmt.Fprintln(os.Stderr, "usage: comparison naive|spinq")
		os.Exit(1)
	}
}

// chunkedWriter splits every Write into small pieces instead of handing the
// underlying writer one atomic call, the way a bufio.Writer with a small
// buffer or a streamed HTTP body would. It exists to make the race below
// realistic rather than staged: with it, a line can genuinely still be
// mid-flight - no trailing newline written yet - when the spinner's next
// tick lands, instead of that only happening if we hand-picked one line to
// leave incomplete.
type chunkedWriter struct {
	w io.Writer
}

func (cw chunkedWriter) Write(p []byte) (int, error) {
	const chunkSize = 8
	for i := 0; i < len(p); i += chunkSize {
		end := min(i+chunkSize, len(p))
		if _, err := cw.w.Write(p[i:end]); err != nil {
			return i, err
		}
		time.Sleep(8 * time.Millisecond)
	}
	return len(p), nil
}

// runNaive hand-rolls a spinner on stderr - no shared writer, no
// coordination with the stdout writes happening alongside it on the main
// goroutine. It clears the line before redrawing, same as most hand-rolled
// spinners do to avoid trailing garbage from a shorter frame - which is
// exactly what lets it erase a line the main goroutine just printed to
// stdout: the clear targets "the current row" with no idea that row now
// holds real output instead of the spinner's own last frame. That's the
// exact bug spinq's Pair exists to fix: two streams, same terminal,
// nothing serializing the writes between them.
func runNaive() {
	done := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		i := 0
		for {
			select {
			case <-done:
				return
			default:
			}
			fmt.Fprintf(os.Stderr, "\r\033[K%s Running (%.1fs)", spinq.DotsStates[i%len(spinq.DotsStates)], time.Since(start).Seconds())
			i++
			time.Sleep(45 * time.Millisecond)
		}
	}()

	out := chunkedWriter{os.Stdout}
	for idx, s := range steps {
		time.Sleep(s.dur)
		fmt.Fprintf(out, "✓ %2d %s\n", idx+1, s.name)
	}
	close(done)
	wg.Wait()
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stdout, "\n%s✓ %d/%d passed%s\n", spinq.Green, len(steps), len(steps), spinq.ResetStyle)
}

// runSpinq runs the identical pipeline, written through the same chunked,
// gappy writer as runNaive, but targeting pair.Standard instead of a raw
// os.Stdout. spinq's Pair locks around every single Write - clearing the
// spinner, writing, redrawing (see writerReal.Write) - so the same partial,
// no-newline-yet chunk that lets the naive spinner erase a line is just
// business as usual here.
func runSpinq() {
	pair, err := spinq.JustStart()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start spinner: %s\n", err)
		os.Exit(1)
	}
	defer pair.Spinner.Close()

	out := chunkedWriter{pair.Standard}
	for idx, s := range steps {
		time.Sleep(s.dur)
		fmt.Fprintf(out, "✓ %2d %s\n", idx+1, s.name)
	}

	pair.Spinner.Stop()
	fmt.Fprintf(pair.Standard, "\n%s✓ %d/%d passed%s\n", spinq.Green, len(steps), len(steps), spinq.ResetStyle)
}
