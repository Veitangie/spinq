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
		spinq.DynamicBarRender(barWidth, spinq.BarWithThinPreset()),
		spinq.FractRender("/"),
		spinq.PercentRender(),
	)
	getFrame := spinq.Progress(func() (int, int) { return int(count.Load()), total }, render)

	// No resize option: DynamicBarRender fits the bar itself, so every clear
	// stays one row. See the "On resizing" section of the README for when
	// you'd want spinq.WrapWithResizeDetection instead - and why you usually
	// don't.
	p, err := spinq.WrapOS(ctx, getFrame, spinq.Every(100*time.Millisecond))
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
