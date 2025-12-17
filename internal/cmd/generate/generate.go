// Package generate provides code generation subcommands.
package generate

import (
	"fmt"
	"os"

	"github.com/yacchi/jubako/internal/cmd/generate/paths"
)

// Run executes the generate subcommand.
func Run(args []string) error {
	if len(args) < 1 {
		PrintHelp()
		return fmt.Errorf("missing subcommand")
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "paths":
		return paths.Run(subargs)
	case "help", "-h", "--help":
		PrintHelp()
		return nil
	default:
		PrintHelp()
		return fmt.Errorf("unknown subcommand: %s", subcmd)
	}
}

// PrintHelp prints help for the generate command.
func PrintHelp() {
	fmt.Fprintln(os.Stderr, `jubako generate - Code generation commands

Usage:
  go tool jubako generate <subcommand> [arguments]

Subcommands:
  paths       Generate JSONPointer path constants and functions from struct types

Use "go tool jubako generate <subcommand> -h" for more information.`)
}
