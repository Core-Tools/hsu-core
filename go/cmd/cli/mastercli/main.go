package main

import (
	"fmt"
	"os"

	sprintflogging "github.com/core-tools/hsu-core/pkg/logging/sprintf"

	flags "github.com/jessevdk/go-flags"
)

type flagOptions struct {
	ServerPath string `long:"server" description:"path to the server executable"`
	AttachPort int    `long:"port" description:"port to attach to the server"`
}

func logPrefix(module string) string {
	return fmt.Sprintf("module: %s-client , ", module)
}

func main() {
	var opts flagOptions
	var argv []string = os.Args[1:]
	var parser = flags.NewParser(&opts, flags.HelpFlag)
	var err error
	_, err = parser.ParseArgs(argv)
	if err != nil {
		fmt.Printf("Command line flags parsing failed: %v", err)
		os.Exit(1)
	}

	logger := sprintflogging.NewStdSprintfLogger()

	logger.Infof("opts: %+v", opts)

	if opts.ServerPath == "" && opts.AttachPort == 0 {
		fmt.Println("Server path or attach port is required")
		os.Exit(1)
	}

	logger.Infof("Starting...")

	logger.Infof("Done")
}
