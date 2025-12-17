// Package paths provides the "generate paths" subcommand.
package paths

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Options holds the command-line options for the paths generator.
type Options struct {
	TypeName    string
	TagName     string
	Output      string
	PackageName string
}

// Run executes the paths generation command.
func Run(args []string) error {
	fs := flag.NewFlagSet("generate paths", flag.ExitOnError)

	var opts Options
	fs.StringVar(&opts.TypeName, "type", "", "target struct type name (required)")
	fs.StringVar(&opts.TagName, "tag", "json", "tag name for field resolution")
	fs.StringVar(&opts.Output, "output", "", "output file path (default: stdout)")
	fs.StringVar(&opts.PackageName, "package", "", "output package name (default: same as input)")

	fs.Usage = func() {
		printHelp()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if opts.TypeName == "" {
		printHelp()
		return fmt.Errorf("-type flag is required")
	}

	remaining := fs.Args()
	if len(remaining) != 1 {
		printHelp()
		return fmt.Errorf("exactly one source file is required")
	}

	sourceFile := remaining[0]

	return runGenerate(sourceFile, opts)
}

func runGenerate(sourceFile string, opts Options) error {
	// Parse the source file
	pkg, structType, err := parseSourceFile(sourceFile, opts.TypeName)
	if err != nil {
		return fmt.Errorf("failed to parse source file: %w", err)
	}

	// Determine package name
	pkgName := opts.PackageName
	if pkgName == "" {
		pkgName = pkg.Name
	}

	// Determine output file name
	outputFile := opts.Output
	if outputFile == "" {
		outputFile = defaultOutputFile(sourceFile)
	}

	// Analyze the struct
	analysis, err := analyzeStruct(structType, opts.TagName)
	if err != nil {
		return fmt.Errorf("failed to analyze struct: %w", err)
	}

	// Generate the code
	code, err := generateCode(analysis, GeneratorConfig{
		PackageName: pkgName,
		TypeName:    opts.TypeName,
		SourceFile:  sourceFile,
		TagName:     opts.TagName,
		Output:      outputFile,
	})
	if err != nil {
		return fmt.Errorf("failed to generate code: %w", err)
	}

	// Write output
	if err := os.WriteFile(outputFile, code, 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}
	fmt.Fprintf(os.Stderr, "generated %s\n", outputFile)

	return nil
}

// defaultOutputFile returns the default output file name based on the source file.
// e.g., "config.go" -> "config_paths.go"
func defaultOutputFile(sourceFile string) string {
	dir := filepath.Dir(sourceFile)
	base := filepath.Base(sourceFile)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, name+"_paths"+ext)
}

func printHelp() {
	fmt.Fprintln(os.Stderr, `jubako generate paths - Generate JSONPointer path constants and functions

Usage:
  go tool jubako generate paths [options] <source-file>

Options:
  -type string      Target struct type name (required)
  -tag string       Tag name for field resolution (default "json")
  -output string    Output file path (default: <source>_paths.go)
  -package string   Output package name (default: same as input)

Examples:
  go tool jubako generate paths -type AppConfig config.go
  go tool jubako generate paths -type AppConfig -tag yaml config.go

For use with go:generate:
  //go:generate go tool jubako generate paths -type AppConfig`)
}
