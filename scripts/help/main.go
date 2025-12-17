package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	file, err := os.Open("Makefile")
	if err != nil {
		return fmt.Errorf("failed to open Makefile: %w", err)
	}
	defer file.Close()

	var targets []struct {
		Name string
		Desc string
	}

	scanner := bufio.NewScanner(file)
	var prevLine string
	targetRegex := regexp.MustCompile(`^([a-zA-Z0-9_-]+):`)

	for scanner.Scan() {
		line := scanner.Text()

		// Check for target definition
		if matches := targetRegex.FindStringSubmatch(line); len(matches) > 1 {
			targetName := matches[1]

			// Check if previous line is a help comment (starts with "## ")
			if strings.HasPrefix(prevLine, "## ") {
				desc := strings.TrimSpace(strings.TrimPrefix(prevLine, "## "))
				targets = append(targets, struct {
					Name string
					Desc string
				}{Name: targetName, Desc: desc})
			}
		}

		prevLine = line
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading Makefile: %w", err)
	}

	// Output
	fmt.Println("\nUsage:")
	fmt.Println("  make \033[36m<target>\033[0m")
	fmt.Println("\nTargets:")

	// minwidth, tabwidth, padding, padchar, flags
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, t := range targets {
		// \033[36m... \033[0m for cyan color
		fmt.Fprintf(w, "  \033[36m%s\033[0m\t%s\n", t.Name, t.Desc)
	}
	w.Flush()

	return nil
}
