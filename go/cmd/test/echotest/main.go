package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	flags "github.com/jessevdk/go-flags"
)

type flagOptions struct {
	RunDuration int `long:"run-duration" description:"Duration in seconds to run the master (debug feature)"`
	MemoryMB    int `long:"memory-mb" description:"Memory in Megabytes to allocate (debug feature)"`
}

func main() {
	var opts flagOptions
	var argv []string = os.Args[1:]
	var parser = flags.NewParser(&opts, flags.HelpFlag)
	var err error
	_, err = parser.ParseArgs(argv)
	if err != nil {
		fmt.Printf("Command line flags parsing failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Running Echotest, opts: %+v...\n", opts)

	runDuration := opts.RunDuration
	memoryMB := opts.MemoryMB

	ctx := context.Background()

	if runDuration > 0 {
		fmt.Printf("Using RUN DURATION of %d seconds\n", runDuration)
		ctx, _ = context.WithTimeout(ctx, time.Duration(runDuration)*time.Second)
	}

	var s []byte
	if memoryMB > 0 {
		fmt.Printf("Using MEMORY MB of %d Megabytes\n", memoryMB)
		s = make([]byte, memoryMB*1024*1024)
	}
	for i := 0; i < len(s); i++ {
		s[i] = 0
	}

	// Enable signal handling
	sig := make(chan os.Signal, 1)
	if runtime.GOOS == "windows" {
		signal.Notify(sig) // Unix signals not implemented on Windows
	} else {
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	}

	fmt.Printf("Echotest is ready, starting workers...\n")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		time.Sleep(2 * time.Second)

		fmt.Printf("Echotest is fully operational\n")
	}()

	// Wait for graceful shutdown or timeout
	select {
	case receivedSignal := <-sig:
		fmt.Printf("Echotest received signal: %v\n", receivedSignal)
		if runtime.GOOS == "windows" {
			if receivedSignal != os.Interrupt {
				fmt.Printf("Wrong signal received: got %q, want %q\n", receivedSignal, os.Interrupt)
				os.Exit(42)
			}
		}
	case <-ctx.Done():
		fmt.Printf("Echotest timed out\n")
	}

	fmt.Printf("Echotest stopped\n")
}
