# spinq

**S**imple s**PIN**ner tool**Q**it.

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/Veitangie/spinq/graph/badge.svg)](https://codecov.io/gh/Veitangie/spinq)
[![CI](https://github.com/Veitangie/spinq/actions/workflows/ci.yml/badge.svg)](https://github.com/Veitangie/spinq/actions/workflows/ci.yml)
[![Test Efficacy](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/Veitangie/assets/main/badges/spinq/gremlins.json)](https://github.com/Veitangie/spinq/actions/workflows/gremlins.yml)
![Release Version](https://img.shields.io/github/v/release/Veitangie/spinq?include_prereleases&logo=github)
[![Go Reference](https://pkg.go.dev/badge/veitangie.dev/spinq.svg)](https://pkg.go.dev/veitangie.dev/spinq)

A small, focused, actor-based terminal spinner and progress-bar library for Go - no TUI framework underneath, just the spinner, the bar, and resize-aware rendering.

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
	defer p.Spinner.Close()
	stdout, stderr := p.Standard, p.Spinner
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
<summary>Progress bar example (100 concurrent workers, one shared bar)</summary>

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
			func() (int, int) { return int(count.Load()), 100 },
			spinq.SmoothBarRender(22).
				Join(" ", spinq.FractRender("/"))),
		spinq.Every(100*time.Millisecond),
	)
	if err != nil {
		fmt.Printf("Failed to start spinner: %s\n", err.Error())
		os.Exit(1)
	}
	defer p.Spinner.Close()

	stdout, stderr := p.Standard, p.Spinner
	stderr.Start(context.Background())

	latch := &sync.WaitGroup{}
	wg := &sync.WaitGroup{}
	latch.Add(1)
	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			latch.Wait()
			time.Sleep(time.Duration(rand.Int63n(5000 * int64(time.Millisecond))))
			fmt.Fprintf(stdout, "Worker %d is doing stuff\n", i)
			if i%10 == 0 {
				fmt.Fprintf(stderr, "%sWorker %d FAILED%s\n", spinq.Red, i, spinq.ResetStyle)
			}
			count.Add(1)
		}(i)
	}

	latch.Done()
	wg.Wait()
	stderr.StopNoClear(" " + spinq.Green + "✓" + spinq.ResetStyle + " Done\n")
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

	resizeOpt, getWidth, err := spinq.WrapDefaultResizeDetection(ctx)
	if err != nil {
		fmt.Printf("Failed to detect terminal width: %s\n", err.Error())
		os.Exit(1)
	}

	barWidth := spinq.Offset(getWidth, -21)

	const total = 1000
	var count atomic.Int64
	render := spinq.JoinRender(" ",
		spinq.DynamicBarRender(barWidth, spinq.BarWithThinPreset()),
		spinq.FractRender("/"),
		spinq.PercentRender(),
	)
	getFrame := spinq.Progress(func() (int, int) { return int(count.Load()), total }, render)

	p, err := spinq.WrapOS(ctx, getFrame, spinq.Every(100*time.Millisecond), resizeOpt)
	if err != nil {
		fmt.Printf("Failed to start spinner: %s\n", err.Error())
		os.Exit(1)
	}
	defer p.Spinner.Close()

	fmt.Fprintf(p.Standard, "%d cols wide, bar gets %d\n", getWidth(), barWidth())
	if err := p.Spinner.Start(ctx); err != nil {
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

	p.Spinner.StopNoClear(" " + spinq.Green + "done" + spinq.ResetStyle + "\n")
}
```

</details>

All three examples live in [`examples/`](examples/) and run as-is with `go run .`.

## Why spinq exists

Every existing Go spinner library I looked at made me choose between "too heavy" and "actually going to corrupt my output eventually." Specifically:

- **Too heavy.** Some pull in sizable dependency trees, or arrive bundled as part of a larger TUI framework I didn't ask for. spinq imports four packages at runtime, each earning its keep: [`go-colorable`](https://github.com/mattn/go-colorable) and [`go-isatty`](https://github.com/mattn/go-isatty) for Windows ANSI support and terminal detection, [`displaywidth`](https://github.com/clipperhouse/displaywidth) (built on [`uax29`](https://github.com/clipperhouse/uax29)'s grapheme-cluster segmentation) for correct grapheme-aware cell-width math (so bars, dividers, and cropped frames line up correctly with wide/multi-byte glyphs and ANSI codes), and `golang.org/x/term` for terminal-size queries. `go.mod` also lists `creack/pty` as a direct dependency - that's test-only tooling for the PTY-backed test suite, never imported by spinq itself. Everything else is standard library.

- **Corrupting the other stream.** Most libraries only ever manage the stream they spin on, and never account for the fact that your program's *other* stream shares the same physical terminal. Print to stdout while a spinner animates on stderr, and you can still get visual corruption on screen, or even get your written data deleted off the screen. spinq avoids this by giving you two independently addressable writers instead of one: `pair.Standard` and `pair.Spinner` can point at different streams (or the same one), stay separately pipeable/redirectable, and spinq coordinates between them internally instead of only managing the one it spins on. At the time of writing (August 2026) I didn't manage to find a single lightweight library that prevented this risk.

If none of that matters for your use case, you probably don't need spinq - plenty of other great options exist. If it does, spinq was made to solve exactly these problems.

## What spinq does not do

spinq is meant to stay small and easy to use, so it comes with some restrictions:

- **No raw/true TTY mode.** spinq never puts the terminal into raw mode, never reads input, and isn't a TUI framework. It's a simple ANSI-writing `io.Writer`. For when you need something to just spin.

- **No automatic width detection.** Bar widths are explicit `int` arguments by default - spinq never queries the terminal size on its own. If you want responsive bars, that's an explicit opt-in - for the common case (size against `os.Stderr`, real `SIGWINCH`), `WrapDefaultResizeDetection`/`DefaultResizeDetection` wire it up in one call and hand back the `getWidth` too (see the [responsive bar example](#see-it-in-action) above). To wire it up yourself: a `getWidth` (see `WidthFromFile`), wrapped in `CachedGetWidth` so the actual syscall only happens on a real resize instead of on every frame, shaped with `Portion`/`Offset`/`Clamp` as needed (half the terminal, minus room for a label, bounded to a sane range), and passed to `DynamicBarRender`/`DynamicSmoothBarRender` - or `Dynamic` directly, for anything that isn't a bar.

- **No multiline or multi-bar dashboards.** spinq can only manage one line. If you want several concurrent progress bars stacked on screen, spinq is not a good choice.

- **Minimal terminal capability negotiation.** Detection is `isatty`-based (a real terminal vs. redirected/piped output, including Cygwin/MSYS2 ptys) rather than terminfo/termcap parsing or fallback rendering for genuinely non-ANSI terminals - though output is wrapped through `go-colorable` on Windows, so ANSI sequences render correctly there too instead of printing as literal escape-code garbage.

## When something else is a better fit

spinq is scoped deliberately narrow - see above. That's not the right shape for every job, so here's when each popular alternative is a better pick: what it has that spinq doesn't, and what of spinq's you'd be giving up (or simply don't need) to get it.

- **[briandowns/spinner](https://github.com/briandowns/spinner)** - you want a spinner only, nothing else, at the smallest possible dependency footprint, with 90+ built-in character sets to pick from. You don't need a progress bar, resize-aware rendering, or spinq's stdout/stderr write coordination.

- **[yacspin](https://github.com/theckman/yacspin)** - same spinner-only scope as briandowns/spinner, with more built-in behavior: automatic padding so the animation's width doesn't shift surrounding text, and named success/failure stop methods instead of composing your own final message via `StopWith`. You still don't need a progress bar or spinq's stdout/stderr coordination.

- **[mpb](https://github.com/vbauerster/mpb)** - you need more than one progress bar on screen at once - a parallel download manager, several workers each with their own bar. spinq explicitly manages a single line only; mpb is built around multiple bars added and removed dynamically, with decorator column widths kept in sync across all of them.

- **[cheggaaa/pb](https://github.com/cheggaaa/pb)** - similar multi-bar territory (it calls this a pool), plus built-in `io.Reader`/`io.Writer` wrapping so a bar tracks bytes read or written from a stream without you wiring up a counter yourself, and byte-unit formatting (KiB/MiB/...) out of the box.

- **[schollz/progressbar](https://github.com/schollz/progressbar)** - you want a single bar capable of turning itself into a spinner automatically when the total is unknown. You don't need spinq's stdout/stderr coordination or its smaller footprint - schollz/progressbar runs roughly ~4.7x heavier (see the [full comparison](#footprint-comparison) below for the rest of these).

- **[pterm](https://github.com/pterm/pterm)** - a spinner or bar is only one piece of what you need. pterm is a full styled-console toolkit - tables, trees, prompts, select menus, panels, charts - and you want one consistent look across all of it rather than pairing spinq with separate libraries for the rest.

- **[bubbletea](https://github.com/charmbracelet/bubbletea)** (with [bubbles](https://github.com/charmbracelet/bubbles) for its spinner/progress components) - you're building an actual interactive terminal application: keyboard/mouse input, multiple views, real application state - not decorating a linear CLI's output while it runs in the background. spinq is deliberately not a TUI framework (see above); bubbletea is exactly that.

(Footprint delta above is stripped-binary size, `-ldflags="-s -w"`, measured against an empty Go program on the same toolchain - directional, not a promise that will hold across every version of either library.)

If what you want is a spinner and/or a single-line progress bar, coordinated with your program's normal stdout/stderr output, without adopting a TUI framework - that's the case spinq is built for.

<details id="footprint-comparison">
<summary>Full size comparison, if you want the numbers behind "roughly Nx heavier"</summary>

Same methodology as the footnote above (stripped-binary delta over an empty Go program), run across every library mentioned in this section, each at its latest tagged release. This isn't cherry-picked to flatter spinq - one of the alternatives below is genuinely smaller (a bare spinner, nothing else):

| library | version | scope | delta | vs. spinq |
|---|---|---|---:|---:|
| [briandowns/spinner](https://github.com/briandowns/spinner) | v1.23.2 | bare spinner only | 376 KB | 0.58x |
| **spinq** | **v1.0.0-rc.12** | spinner + bar + resize-aware + grapheme-correct | **644 KB** | **1.00x** |
| [yacspin](https://github.com/theckman/yacspin) | v0.13.12 | bare spinner only, configurable | 844 KB | 1.31x |
| [mpb](https://github.com/vbauerster/mpb) | v8.16.0 | dedicated multi-progress-bar library | 1020 KB | 1.58x |
| [pterm](https://github.com/pterm/pterm) | v0.12.83 | full styled-console toolkit | 1448 KB | 2.25x |
| [bubbletea](https://github.com/charmbracelet/bubbletea) | v1.3.10 (+ [bubbles](https://github.com/charmbracelet/bubbles) v1.0.0) | Elm-architecture TUI framework | 1736 KB | 2.70x |
| [cheggaaa/pb](https://github.com/cheggaaa/pb) | v3.2.1 | dedicated progress-bar library | 2232 KB | 3.47x |
| [schollz/progressbar](https://github.com/schollz/progressbar) | v3.19.1 | dedicated progress-bar library | 3000 KB | 4.66x |

Read this as directional, not a permanent ranking - each library's own dependencies shift over time, and a newer or older version of any of these could land differently; the version column pins down exactly what was measured, so this can be reproduced or checked against by anyone. Measured August 2026, same Go toolchain (go1.27.0) throughout.

</details>

## Install

```sh
go get veitangie.dev/spinq
```

Requires the Go version declared in `go.mod`.

## Versioning

spinq follows semantic versioning, judged strictly from the calling code's perspective:

- **Patch** - invisible to any consumer, even if it touches exported types under the hood. Fixing undefined behavior (e.g. `SigwinchFromPoller` returning a literal `nil` channel for a non-positive duration - unusable the moment a caller ranges over it - instead of an already-closed one, matching every other degenerate-input case in the package), adding new internal implementation, hardening against a crash that should never have been reachable - all patches.
- **Minor** - additive: everything that already compiled keeps compiling and behaving the same. A new optional parameter via `...T`, a new method, a new exported function or type.
- **Major** - anything that breaks compilation for existing callers - a changed signature on an existing exported function or method - or breaks an existing behavioral contract even without a signature change, such as a guarantee spinq previously made and no longer keeps. As Go modules require, a major bump also gets a new import path (`veitangie.dev/spinq/v2`, and so on).

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
	defer pair.Spinner.Close()

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
	spinq.WithEvery(50 * time.Millisecond),
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
defer pair.Spinner.Close()
```

`BarRender` ships a handful of presets (`BarWithRoundedPreset`, `BarWithShadePreset`, `BarWithDotPreset`, `BarWithMinimalPreset`, `BarWithThinPreset`), or takes functional options (`BarWithFull`, `BarWithDivider`, `BarWithDirection`, ...) to build your own. `SmoothBarRender` (sub-cell precision, for smoother fill) has its own preset set instead (`SmoothWithSnakePreset`, `SmoothWithBraillePreset`, `SmoothWithPiePreset`, `SmoothWithDotPreset`, `SmoothWithShadePreset`), plus the matching `SmoothWith*` functional options.

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

- `WrapPair(ctx, main, spinner, getFrame, ticker, opts...)`: the primitive. Takes any two `io.Writer`s, no TTY detection at all.

- `WrapFilePair(ctx, main, spinner *os.File, ..., opts...)`: adds the character-device check, falling back to a passthrough for non-terminal files.

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

If `getFrame` itself also needs that same `getWidth` - to size a `DynamicBarRender`, for instance - resolve both together with `WrapDefaultResizeDetection` (or `DefaultResizeDetection` at the `JustStart` layer) instead of wiring `WrapWithDefaultResizeDetection` and a separate width source up by hand; see the [responsive bar example](#see-it-in-action).

## Staying resilient across write errors

A write failure (a resized/flaky terminal, a closed pipe) auto-stops the spinner - `Stop`/`StopWith`/`StopNoClear` become no-ops until `Start` is called again. `pair.Spinner.Err()` also delivers a `PanicError` whenever a `FrameFunc` call panics, but that case is different: the panic is recovered, that one frame is just skipped, and the spinner is never stopped by it. A long-running program that wants to keep drawing across a write failure should range over `pair.Spinner.Err()` in the background and call `Start` again on each delivery - harmless to do for a `PanicError` delivery too, since `Start` on an already-running spinner is a no-op:

```go
go func() {
	for range pair.Spinner.Err() {
		err := pair.Spinner.Start(ctx)
		if errors.Is(err, spinq.ErrClosed) {
			return // the Writer itself was closed - stop retrying
		}
		// any other error just means this one restart attempt failed;
		// keep waiting for the next delivery
	}
}()
```

## Design

spinq is actor-based: a single goroutine owns all spinner state, and every public method talks to it over a channel. The one exception worth knowing: `Start`'s initial frame, `StopNoClear`'s final frame, and a running `Set`'s redraw all fetch synchronously inside the actor, so a slow `FrameFunc` there blocks that call, any other `Start`/`Stop*`/`Set`/`Close` made concurrently on the same `Writer`, and `Close` itself - a plain `Write` is unaffected. A panic inside `FrameFunc` is recovered, reported on `Err()` as a `PanicError`, and never crashes the process or stops the spinner.

`Close` joins the actor's own goroutine, but not every short-lived goroutine spinq spawns along the way (a per-`Start` context watcher, the tail of a tick-triggered fetch) - those are cancelled, not joined, so nothing guarantees they've exited yet, though none do user-visible work by that point.

See [pkg.go.dev](https://pkg.go.dev/veitangie.dev/spinq) for the full API reference.

## License

Apache 2.0 - see [LICENSE](LICENSE).
