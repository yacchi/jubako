// Command extract-readme extracts runnable Go code blocks from README.md.
//
// It parses the README using goldmark and extracts code blocks that:
//   - Are fenced with ```go
//   - Contain "package main" and "func main()"
//
// Each extracted block is written to examples/.readme/block_NNN/main.go
// where NNN is the starting line number of the code block.
//
// Usage:
//
//	go run ./scripts/extract-readme
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Find project root (where README.md is located)
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	readmePath := filepath.Join(projectRoot, "README.md")
	outputDir := filepath.Join(projectRoot, "examples", ".readme")

	// Read README.md
	source, err := os.ReadFile(readmePath)
	if err != nil {
		return fmt.Errorf("failed to read README.md: %w", err)
	}

	// Parse markdown
	md := goldmark.New()
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	// Extract Go code blocks
	blocks := extractGoCodeBlocks(doc, source)

	// Filter runnable blocks
	runnableBlocks := filterRunnableBlocks(blocks)

	if len(runnableBlocks) == 0 {
		fmt.Println("No runnable README examples found.")
		return nil
	}

	// Clean and recreate output directory
	if err := os.RemoveAll(outputDir); err != nil {
		return fmt.Errorf("failed to clean output directory: %w", err)
	}

	// Write each block to a separate directory
	for _, block := range runnableBlocks {
		blockDir := filepath.Join(outputDir, fmt.Sprintf("block_%03d", block.Line))
		if err := os.MkdirAll(blockDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", blockDir, err)
		}

		mainFile := filepath.Join(blockDir, "main.go")
		if err := os.WriteFile(mainFile, []byte(block.Code), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", mainFile, err)
		}

		fmt.Printf("  Extracted: block_%03d (line %d)\n", block.Line, block.Line)
	}

	fmt.Printf("README code extraction complete: %d runnable example(s)\n", len(runnableBlocks))
	return nil
}

// CodeBlock represents an extracted code block.
type CodeBlock struct {
	Line int    // Starting line number (1-based)
	Code string // Code content
}

// extractGoCodeBlocks walks the AST and extracts Go fenced code blocks.
func extractGoCodeBlocks(doc ast.Node, source []byte) []CodeBlock {
	var blocks []CodeBlock

	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		fenced, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}

		// Check if it's a Go code block
		lang := string(fenced.Language(source))
		if lang != "go" {
			return ast.WalkContinue, nil
		}

		// Extract code content
		var buf bytes.Buffer
		lines := fenced.Lines()
		for i := 0; i < lines.Len(); i++ {
			line := lines.At(i)
			buf.Write(line.Value(source))
		}

		// Get line number (1-based)
		// The segment start gives us the byte offset; we need to convert to line number
		startLine := countLines(source, fenced.Lines().At(0).Start) + 1

		blocks = append(blocks, CodeBlock{
			Line: startLine,
			Code: buf.String(),
		})

		return ast.WalkContinue, nil
	})

	return blocks
}

// countLines counts newlines before the given byte offset.
func countLines(source []byte, offset int) int {
	return bytes.Count(source[:offset], []byte("\n"))
}

// filterRunnableBlocks filters blocks that have both "package main" and "func main()".
func filterRunnableBlocks(blocks []CodeBlock) []CodeBlock {
	packageMainRe := regexp.MustCompile(`(?m)^package\s+main\s*$`)
	funcMainRe := regexp.MustCompile(`(?m)^func\s+main\s*\(\s*\)`)

	var runnable []CodeBlock
	for _, block := range blocks {
		if packageMainRe.MatchString(block.Code) && funcMainRe.MatchString(block.Code) {
			runnable = append(runnable, block)
		}
	}
	return runnable
}

// findProjectRoot finds the project root by looking for go.mod.
func findProjectRoot() (string, error) {
	// Start from current directory
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up until we find go.mod with "module github.com/yacchi/jubako"
	for {
		gomodPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(gomodPath); err == nil {
			if strings.Contains(string(data), "module github.com/yacchi/jubako\n") {
				return dir, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("could not find project root (go.mod with github.com/yacchi/jubako)")
}
