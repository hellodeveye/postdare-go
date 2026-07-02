package main

import (
	"fmt"
	"io"
	"os"

	"github.com/hellodeveye/postdare-go/internal/cli"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	command := "serve"
	commandArgs := []string{}
	if len(args) > 0 {
		command = args[0]
		commandArgs = args[1:]
	}

	switch command {
	case "serve":
		if len(commandArgs) > 0 {
			usage(stderr)
			return 2
		}
		if err := cli.Serve(version); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "mcp":
		if len(commandArgs) > 0 {
			usage(stderr)
			return 2
		}
		if err := cli.MCP(os.Stdin, stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "version":
		if len(commandArgs) > 0 {
			usage(stderr)
			return 2
		}
		fmt.Fprintln(stdout, version)
		return 0
	default:
		usage(stderr)
		return 2
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  postdare-go [serve]
  postdare-go mcp
  postdare-go version`)
}
