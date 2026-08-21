# spinq

**S**imple s**PIN**ner tool**Q**it.

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/Veitangie/spinq/graph/badge.svg)](https://codecov.io/gh/Veitangie/spinq)
[![CI](https://github.com/Veitangie/spinq/actions/workflows/ci.yml/badge.svg)](https://github.com/Veitangie/spinq/actions/workflows/ci.yml)
![Release Version](https://img.shields.io/github/v/release/Veitangie/spinq?include_prereleases&logo=github)
[![Go Reference](https://pkg.go.dev/badge/veitangie.dev/spinq.svg)](https://pkg.go.dev/veitangie.dev/spinq)

A lightweight, actor-based terminal spinner and progress-bar library for Go.

## See it in action

![Simple spinner demo](examples/simple/simple.gif)

```go
package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"veitangie.dev/spinq"
)

func main() {
	p, err := spinq.JustStart()
	if err != nil {
		fmt.Printf("Failed to start spinner: %s\n", err.Error())
		os.Exit(1)
	}
	defer p.Close()
	stdout, stderr := p.Standard, p.Spinny
	defer stderr.StopWith("All done!\n")
	fmt.Fprintln(stdout, "Going to sleep for 3 seconds")
	go func() {
		time.Sleep(time.Duration(rand.Intn(3)) * time.Second)
		fmt.Fprintln(stderr, "This is an error on stderr")
	}()
	time.Sleep(3 * time.Second)
}
```

<details>
<summary>Progress bar example (1000 concurrent workers, one shared bar)</summary>

![Progress bar demo](examples/progress-bar/progress-bar.gif)

```go
package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"veitangie.dev/spinq"
)

func main() {
	count := atomic.Int64{}

	p, err := spinq.WrapOS(
		context.Background(),
		spinq.Progress(
			func() (int, int) { return int(count.Load()), 1000 },
			spinq.SmoothBarRender(12).
				Join(" ", spinq.FractRender("/"))),
		spinq.Every(100*time.Millisecond),
	)
	if err != nil {
		fmt.Printf("Failed to start spinner: %s\n", err.Error())
		os.Exit(1)
	}
	defer p.Close()

	stdout, stderr := p.Standard, p.Spinny
	stderr.Start(context.Background())

	latch := &sync.WaitGroup{}
	wg := &sync.WaitGroup{}
	latch.Add(1)
	for i := range 1000 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			latch.Wait()
			time.Sleep(time.Duration(rand.Int63n(5000 * int64(time.Millisecond))))
			fmt.Fprintf(stdout, "Worker %d is doing stuff\n", i)
			if i%10 == 0 {
				fmt.Fprintf(stderr, "%sWorker %d FAILED%s\n", spinq.Red, i, spinq.ResetColor)
			}
			count.Add(1)
		}(i)
	}

	latch.Done()
	wg.Wait()
	p.Spinny.StopNoClear(" " + spinq.Green + "✓" + spinq.ResetColor + " Done\n")
}
```

</details>

<details>
<summary>Responsive bar example (same command, two terminal widths)</summary>

![Responsive bar demo, narrow terminal](examples/dynamic/dynamic-narrow.gif)
![Responsive bar demo, wide terminal](examples/dynamic/dynamic-wide.gif)

```go
package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"veitangie.dev/spinq"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	getWidth, err := spinq.DefaultGetWidth(ctx)
	if err != nil {
		fmt.Printf("Failed to detect terminal width: %s\n", err.Error())
		os.Exit(1)
	}

	barWidth := spinq.Offset(getWidth, -21)

	const total = 1000
	var count atomic.Int64
	render := spinq.JoinRender(" ",
		spinq.DynamicBarRender(barWidth, spinq.WithThinBarOptions()),
		spinq.FractRender("/"),
		spinq.PercentRender(),
	)
	getFrame := spinq.Progress(func() (int, int) { return int(count.Load()), total }, render)

	p, err := spinq.WrapOS(ctx, getFrame, spinq.Every(100*time.Millisecond),
		spinq.WrapWithResizeDetection(getWidth))
	if err != nil {
		fmt.Printf("Failed to start spinner: %s\n", err.Error())
		os.Exit(1)
	}
	defer p.Close()

	fmt.Fprintf(p.Standard, "%d cols wide, bar gets %d\n", getWidth(), barWidth())
	if err := p.Spinny.Start(ctx); err != nil {
		fmt.Printf("Failed to start spinner: %s\n", err.Error())
		os.Exit(1)
	}

	var wg sync.WaitGroup
	wg.Add(total)
	for range total {
		go func() {
			defer wg.Done()
			time.Sleep(time.Duration(rand.Int63n(2000)) * time.Millisecond)
			count.Add(1)
		}()
	}
	wg.Wait()

	p.Spinny.StopNoClear(" " + spinq.Green + "done" + spinq.ResetColor + "\n")
}
```

</details>

All three examples live in [`examples/`](examples/) and run as-is with `go run .`.

## Why spinq exists

Every existing Go spinner library I looked at made me choose between "too heavy" and "actually going to corrupt my output eventually." Specifically:

- **Too heavy.** Some pull in sizable dependency trees, or arrive bundled as part of a larger TUI framework I didn't ask for. spinq ships six direct dependencies, each earning its keep: [`go-colorable`](https://github.com/mattn/go-colorable) and [`go-isatty`](https://github.com/mattn/go-isatty) for Windows ANSI support and terminal detection, [`displaywidth`](https://github.com/clipperhouse/displaywidth) and [`uax29`](https://github.com/clipperhouse/uax29) for correct grapheme-aware cell-width math (so bars, dividers, and cropped frames line up correctly with wide/multi-byte glyphs and ANSI codes), `golang.org/x/term` for terminal-size queries, and `golang.org/x/sync` for the concurrency primitive that keeps `FrameFunc` calls serialized. Everything else is standard library.

- **Forcing the user to give up the stdout/stderr split.** A lot of spinner libraries want to *own* the output - you print through their writer, or not at all, and the Unix convention of "stdout is data, stderr is status" is not supported. spinq still needs you to write through its writers. But spinq keeps both streams separate and independently addressable: `pair.Standard` and `pair.Spinny` can point at different streams (or the same one), stay separately pipeable/redirectable, and spinq coordinates between them instead of collapsing them into one.

- **Only ever managing stderr.** Some libraries only consider the stream they spin on, and never account for the fact that your program's *other* stream shares the same physical terminal. Print to stdout while a spinner animates on stderr, and you can still get visual corruption on screen, or even get your written data deleted off the screen. spinq's approach of bundling two writers allows it to manage the spinner without risking corruption on the other stream. At the time of writing (August 2026) I didn't manage to find a single lightweight library that prevented this risk.

If none of that matters for your use case, you probably don't need spinq - plenty of other great options exist. If it does, spinq was made to solve exactly these problems.

## What spinq does not do

spinq is meant to be lightweight and easy to use, so it comes with some restrictions:

- **No raw/true TTY mode.** spinq never puts the terminal into raw mode, never reads input, and isn't a TUI framework. It's a simple ANSI-writing `io.Writer`. For when you need something to just spin.

- **No automatic width detection.** Bar widths are explicit `int` arguments by default - spinq never queries the terminal size on its own. If you want responsive bars, that's an explicit opt-in: wire up a `getWidth` (see `WidthFromFile`), wrap it in `CachedGetWidth` so the actual syscall only happens on a real resize instead of on every frame, shape the result with `Portion`/`Offset`/`Clamp` as needed (half the terminal, minus room for a label, bounded to a sane range), and pass it to `DynamicBarRender`/`DynamicSmoothBarRender` - or `Dynamic` directly, for anything that isn't a bar.

- **No multiline or multi-bar dashboards.** spinq can only manage one line. If you want several concurrent progress bars stacked on screen, spinq is not a good choice.

- **Minimal terminal capability negotiation.** Detection is `isatty`-based (a real terminal vs. redirected/piped output, including Cygwin/MSYS2 ptys) rather than terminfo/termcap parsing or fallback rendering for genuinely non-ANSI terminals - though output is wrapped through `go-colorable` on Windows, so ANSI sequences render correctly there too instead of printing as literal escape-code garbage.

## Install

```sh
go get veitangie.dev/spinq
```

Requires the Go version declared in `go.mod`.

## Quick start

```go
package main

import (
	"fmt"
	"time"

	"veitangie.dev/spinq"
)

func main() {
	pair, err := spinq.JustStart()
	if err != nil {
		panic(err)
	}
	defer pair.Close()

	for i := range 5 {
		time.Sleep(400 * time.Millisecond)
		fmt.Fprintf(pair.Standard, "step %d complete\n", i)
	}
}
```

`JustStart` picks sensible defaults - a dots spinner, a "Running (2.3s)"
duration label, a 100ms redraw tick - and falls back to a silent passthrough
automatically if stdout/stderr aren't real terminals (redirected output,
CI). No spinner ever leaks escape codes into a log file.

Customize the label, states, or redraw interval without building a frame
yourself:

```go
pair, err := spinq.JustStart(
	spinq.WithText("Uploading"),
	spinq.WithStates(spinq.ArrowStates),
	spinq.WithDuration(50 * time.Millisecond),
)
```

## Progress bars

```go
var done atomic.Int64
total := 100

render := spinq.JoinRender(" ", spinq.BarRender(30), spinq.PercentRender())
getFrame := spinq.Progress(func() (int, int) {
	return int(done.Load()), total
}, render)

pair, err := spinq.JustStart(spinq.WithFrame(getFrame))
if err != nil {
	panic(err)
}
defer pair.Close()
```

`BarRender`/`SmoothBarRender` (sub-cell precision, for smoother fill) both ship a handful of presets (`WithRoundedBarOptions`, `WithShadeBarOptions`, `WithDotBarOptions`, `WithMinimalBarOptions`, `WithThinBarOptions`), or take functional options (`BarWithFull`, `BarWithDivider`, `BarWithDirection`, ...) to build your own.

## Composing frames

`JustStart`'s own default frame is just ordinary composition of the smaller primitives - nothing it does is unavailable to you:

```go
frame := spinq.Join("",
	spinq.Surrounded(" ", spinq.Simple(spinq.DotsStates), " Running ("),
	spinq.Duration(time.Now),
	spinq.Static(")"),
)
```

`Simple`, `SimpleOnceEvery`, `Random`, `RandomOnceEvery`, `Duration`, `Progress`, `Join`, `Surrounded`, and `Static` all return a plain `FrameFunc` (`func() ([]byte, error)`), so they compose freely. A `FrameFunc` is guaranteed by spinq never to be called concurrently with itself, so its own private state - a counter, an index - never needs its own locking; see the `FrameFunc` doc comment for exactly where that guarantee stops (anything the closure reads that something *else* also writes is still on you to synchronize).

## Lower-level entry points

`JustStart` wraps `WrapOS`, which wraps `WrapFilePair`, which wraps `WrapPair` - each layer adds one piece of default behavior, and each is exported if you need less of it:

- `WrapPair(ctx, main, spinny, getFrame, ticker, opts...)`: the primitive. Takes any two `io.Writer`s, no TTY detection at all.

- `WrapFilePair(ctx, main, spinny *os.File, ..., opts...)`: adds the character-device check, falling back to a passthrough for non-terminal files.

- `WrapOS(ctx, getFrame, ticker, opts...)`: `WrapFilePair` applied to `os.Stdout`/`os.Stderr`, plus a `CI` environment variable check.

- `JustStart(opts...)`: `WrapOS` with defaults and `Start` already called.

Resize detection is off by default but can be opted in. Every layer takes `WrapOptionsFunc`s (`JustStart` takes the equivalent `JustStartOptionsFunc`s), and `WrapWithDefaultResizeDetection` wires up sensible platform defaults with zero configuration at any of them:

```go
pair, err := spinq.WrapOS(
	context.Background(),
	getFrame,
	spinq.Every(100*time.Millisecond),
	spinq.WrapWithDefaultResizeDetection(context.Background()),
)
```

## Design

A single background goroutine (an actor) owns all spinner state and is the only thing that ever touches it. Every public method talks to it over a channel. Overlapping calls to the same `FrameFunc` - from a tick landing while another fetch is still in flight, for instance - are coalesced through a `singleflight.Group`. See [pkg.go.dev](https://pkg.go.dev/veitangie.dev/spinq) for the full API reference.

## License

Apache 2.0 - see [LICENSE](LICENSE).
