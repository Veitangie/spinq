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
	defer p.Close()

	stdout, stderr := p.Standard, p.Spinny
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
				fmt.Fprintf(stderr, "%sWorker %d FAILED%s\n", spinq.Red, i, spinq.ResetColor)
			}
			count.Add(1)
		}(i)
	}

	latch.Done()
	wg.Wait()
	p.Spinny.StopNoClear(" " + spinq.Green + "✓" + spinq.ResetColor + " Done\n")
}
