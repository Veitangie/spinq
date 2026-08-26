// spinq - Simple spinner toolqit
// Copyright (C) 2026 Veitangie
// SPDX-License-Identifier: Apache-2.0
package spinq_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"veitangie.dev/spinq"
)

func ExampleJoin() {
	frame := spinq.Join(", ", spinq.Static("Hello"), spinq.Static("World"))
	out, err := frame()
	if err != nil {
		panic(err)
	}
	fmt.Println(string(out))
	// Output: Hello, World
}

func ExampleJoinRender() {
	render := spinq.JoinRender(" ", spinq.BarRender(10), spinq.PercentRender())
	fmt.Println(string(render(3, 10)))
	// Output: [==>     ]  30%
}

func ExampleJustStart() {
	pair, err := spinq.JustStart()
	if err != nil {
		panic(err)
	}
	defer pair.Spinner.Close() //nolint:errcheck

	for i := range 5 {
		time.Sleep(400 * time.Millisecond)
		fmt.Fprintf(pair.Standard, "step %d complete\n", i) //nolint:errcheck
	}
}

func ExampleJustStart_withOptions() {
	pair, err := spinq.JustStart(
		spinq.WithText("Uploading"),
		spinq.WithStates(spinq.ArrowStates),
		spinq.WithEvery(50*time.Millisecond),
	)
	if err != nil {
		panic(err)
	}
	defer pair.Spinner.Close() //nolint:errcheck
}

func ExampleProgress() {
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
	defer pair.Spinner.Close() //nolint:errcheck
}

func ExampleWrapOS() {
	ctx := context.Background()
	frame := spinq.Join("",
		spinq.Surrounded(" ", spinq.Simple(spinq.DotsStates), " Running ("),
		spinq.Duration(time.Now),
		spinq.Static(")"),
	)

	pair, err := spinq.WrapOS(ctx, frame, spinq.Every(100*time.Millisecond))
	if err != nil {
		panic(err)
	}
	defer pair.Spinner.Close() //nolint:errcheck

	if err := pair.Spinner.Start(ctx); err != nil {
		panic(err)
	}
}
