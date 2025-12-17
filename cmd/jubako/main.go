// Package main provides the jubako CLI tool.
//
// Usage:
//
//	go tool jubako <command> [arguments]
//
// Commands:
//
//	generate    Code generation commands
//	help        Show help for a command
//	version     Show version information
package main

import (
	"fmt"
	"os"

	"github.com/yacchi/jubako/internal/cmd/generate"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "generate":
		if err := generate.Run(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "help":
		if len(args) > 0 {
			printCommandHelp(args[0])
		} else {
			printUsage()
		}
	case "version":
		fmt.Printf("jubako version %s\n", version)
	case "-h", "--help":
		printUsage()
	case "-v", "--version":
		fmt.Printf("jubako version %s\n", version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`jubako - Layered configuration management tool

Usage:
  go tool jubako <command> [arguments]

Commands:
  generate    Code generation commands
  help        Show help for a command
  version     Show version information

Use "go tool jubako help <command>" for more information about a command.`)
}

func printCommandHelp(cmd string) {
	switch cmd {
	case "generate":
		generate.PrintHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(1)
	}
}
