package main

import (
	"context"
	"fmt"
	"os"

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
	return spanemuboost.ServeFromConfig(context.Background(), cfg)
}

func usage() {
	fmt.Fprintf(os.Stderr, `spanemuboost manages long-lived Spanner test backends.

Usage:
  spanemuboost serve <emulator|omni> [--endpoint-file path]

Examples:
  spanemuboost serve omni --endpoint-file /tmp/omni-endpoint.json
  SPANEMUBOOST_ENDPOINT_FILE=/tmp/omni-endpoint.json go test ./...

`)
}
