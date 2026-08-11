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
