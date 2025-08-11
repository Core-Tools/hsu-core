package main

import (
	"fmt"
	"os"

	sprintflogging "github.com/core-tools/hsu-core/pkg/logging/sprintf"

	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/processmanager"

	flags "github.com/jessevdk/go-flags"
)

type flagOptions struct {
	Config      string `long:"config" short:"c" description:"Configuration file path (YAML)" required:"true"`
	EnableLog   bool   `long:"enable-log" description:"Enable log collection (uses defaults if no config)"`
	RunDuration int    `long:"run-duration" description:"Duration in seconds to run (debug feature)"`
}

func logPrefix(module string) string {
	return fmt.Sprintf("module: %s-server , ", module)
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

	sprintfLogger := sprintflogging.NewStdSprintfLogger()

	// Create loggers
	logger := logging.NewLogger(
		logPrefix("hsu-core"), logging.LogFuncs{
			Debugf: sprintfLogger.Debugf,
			Infof:  sprintfLogger.Infof,
			Warnf:  sprintfLogger.Warnf,
			Errorf: sprintfLogger.Errorf,
		})

	err = processmanager.Run(opts.RunDuration, opts.Config, opts.EnableLog, logger)
	if err != nil {
		logger.Errorf("Failed to run: %v", err)
		os.Exit(1)
	}
}
