package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/apstndb/spanemuboost"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		if err := runServe(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "stop":
		if err := runStop(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func runServe(args []string) error {
	cfg, err := spanemuboost.ParseServeArgs(args)
	if err != nil {
		return err
	}
	if cfg.EndpointFile == "" {
		return fmt.Errorf("--endpoint-file is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return spanemuboost.ServeFromConfig(ctx, cfg)
}

func runStop(args []string) error {
	cfg, err := spanemuboost.ParseStopArgs(args)
	if err != nil {
		return err
	}
	return spanemuboost.StopFromConfig(context.Background(), cfg)
}

func usage() {
	fmt.Fprintf(os.Stderr, `spanemuboost manages long-lived Spanner test backends.

Usage:
  spanemuboost serve <emulator|omni> --endpoint-file path [--pid-file path] [--with-default-database]
  spanemuboost stop --endpoint-file path [--pid-file path]

Examples:
  spanemuboost serve omni --endpoint-file /tmp/omni-endpoint.json
  spanemuboost serve omni --endpoint-file /tmp/omni-endpoint.json --with-default-database
  spanemuboost stop --endpoint-file /tmp/omni-endpoint.json
  SPANEMUBOOST_ENDPOINT_FILE=/tmp/omni-endpoint.json go test ./...

The endpoint file is owned by serve: it is written on startup and removed on
exit. Unset SPANEMUBOOST_ENDPOINT_FILE after stopping the lifecycle manager.

`)
}
