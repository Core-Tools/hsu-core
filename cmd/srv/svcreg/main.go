package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/core-tools/hsu-core/pkg/logging"
	sprintflogging "github.com/core-tools/hsu-core/pkg/logging/sprintf"
	"github.com/core-tools/hsu-core/pkg/serviceregistry"

	flags "github.com/jessevdk/go-flags"
)

type options struct {
	Transport  string `long:"transport" description:"Transport type (tcp, unix)" default:"tcp"`
	TCPAddress string `long:"tcp-address" description:"TCP address (for tcp transport)" default:"127.0.0.1:17951"`
	SocketPath string `long:"socket-path" description:"Unix domain socket path (for unix transport)" default:"/tmp/hsu-registry.sock"`
	Verbose    bool   `short:"v" long:"verbose" description:"Verbose logging"`
}

func main() {
	var opts options
	parser := flags.NewParser(&opts, flags.HelpFlag)

	_, err := parser.Parse()
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Failed to parse flags: %v\n", err)
		os.Exit(1)
	}

	// Create logger
	sprintfLogger := sprintflogging.NewStdSprintfLogger()
	logLevel := logging.LogFuncs{
		Debugf: func(format string, args ...interface{}) {},
		Infof:  sprintfLogger.Infof,
		Warnf:  sprintfLogger.Warnf,
		Errorf: sprintfLogger.Errorf,
	}

	if opts.Verbose {
		logLevel.Debugf = sprintfLogger.Debugf
	}

	logger := logging.NewLogger("[svcreg] ", logLevel)

	logger.Infof("Starting HSU Service Registry")
	logger.Infof("Transport: %s", opts.Transport)

	// Create transport config
	var transportConfig serviceregistry.TransportConfig
	switch opts.Transport {
	case "tcp":
		transportConfig = serviceregistry.TransportConfig{
			TransportType: serviceregistry.TransportTCP,
			TCPAddress:    opts.TCPAddress,
		}
		logger.Infof("TCP address: %s", opts.TCPAddress)

	case "unix", "uds":
		transportConfig = serviceregistry.TransportConfig{
			TransportType: serviceregistry.TransportUDS,
			SocketPath:    opts.SocketPath,
			FileMode:      0600,
		}
		logger.Infof("Unix socket path: %s", opts.SocketPath)

	default:
		logger.Errorf("Unsupported transport type: %s", opts.Transport)
		os.Exit(1)
	}

	// Create registry
	registry := serviceregistry.NewRegistry(logger)

	// Create server
	server, err := serviceregistry.NewServer(registry, transportConfig, logger)
	if err != nil {
		logger.Errorf("Failed to create server: %v", err)
		os.Exit(1)
	}

	// Start server
	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		logger.Errorf("Failed to start server: %v", err)
		os.Exit(1)
	}

	logger.Infof("Service registry is running at %s", server.GetAddress())
	logger.Infof("Press Ctrl+C to stop")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Infof("Received shutdown signal, stopping server...")

	// Stop server
	stopCtx := context.Background()
	if err := server.Stop(stopCtx); err != nil {
		logger.Errorf("Error stopping server: %v", err)
		os.Exit(1)
	}

	logger.Infof("Service registry stopped cleanly")
}
