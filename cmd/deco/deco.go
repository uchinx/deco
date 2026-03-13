package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/uchinx/deco/analyzer"
)

func main() {
	sortFlag := flag.String("sort", "file", "sort order: file (by file path and line number), name (by method name)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: deco [flags] [packages]\n\n")
		fmt.Fprintf(os.Stderr, "Finds dead (unused) exported methods in Go code.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  deco ./...          # scan all packages in current module\n")
		fmt.Fprintf(os.Stderr, "  deco ./pkg/mylib    # scan a specific package\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	patterns := flag.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	result, err := analyzer.Analyze(patterns, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	if len(result.UnusedMethods) == 0 {
		os.Exit(0)
	}

	switch *sortFlag {
	case "name":
		sort.Slice(result.UnusedMethods, func(i, j int) bool {
			a, b := result.UnusedMethods[i], result.UnusedMethods[j]
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return comparePositions(a.Position, b.Position) < 0
		})
	default: // "file"
		sort.Slice(result.UnusedMethods, func(i, j int) bool {
			return comparePositions(result.UnusedMethods[i].Position, result.UnusedMethods[j].Position) < 0
		})
	}

	for _, m := range result.UnusedMethods {
		fmt.Printf("%s\t%s.%s\t(%s)\n", m.Position, m.ReceiverType, m.Name, m.Kind)
	}

	os.Exit(1)
}

// comparePositions compares two "file:line" position strings.
// It sorts by file path first, then by line number numerically.
func comparePositions(a, b string) int {
	aFile, aLine := splitPosition(a)
	bFile, bLine := splitPosition(b)
	if aFile != bFile {
		if aFile < bFile {
			return -1
		}
		return 1
	}
	if aLine != bLine {
		if aLine < bLine {
			return -1
		}
		return 1
	}
	return 0
}

func splitPosition(pos string) (string, int) {
	idx := strings.LastIndex(pos, ":")
	if idx < 0 {
		return pos, 0
	}
	line, err := strconv.Atoi(pos[idx+1:])
	if err != nil {
		return pos, 0
	}
	return pos[:idx], line
}
