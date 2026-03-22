// testrunner — runs all tests and prints clean pass/fail output.
// Usage: go run ./cmd/testrunner
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type testEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

type testResult struct {
	name    string
	pkg     string
	passed  bool
	output  []string
}

func main() {
	cmd := exec.Command("go", "test", "./...", "-v", "-json", "-count=1")
	cmd.Dir = projectRoot()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting tests: %v\n", err)
		os.Exit(1)
	}

	// Track results per test
	results := map[string]*testResult{}    // key = pkg::test
	pkgOrder := []string{}                 // track package order
	seenPkgs := map[string]bool{}
	failedOutputs := map[string][]string{} // accumulated output lines per test

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		var ev testEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Test == "" {
			continue // package-level event, skip
		}

		key := ev.Package + "::" + ev.Test

		// Track package order
		if !seenPkgs[ev.Package] {
			seenPkgs[ev.Package] = true
			pkgOrder = append(pkgOrder, ev.Package)
		}

		switch ev.Action {
		case "run":
			results[key] = &testResult{
				name: ev.Test,
				pkg:  ev.Package,
			}
		case "output":
			// Accumulate output lines for this test (used on failure)
			line := strings.TrimRight(ev.Output, "\n")
			if line != "" && !strings.HasPrefix(line, "=== RUN") &&
				!strings.HasPrefix(line, "--- PASS") &&
				!strings.HasPrefix(line, "--- FAIL") {
				failedOutputs[key] = append(failedOutputs[key], line)
			}
		case "pass":
			if r, ok := results[key]; ok {
				r.passed = true
			}
		case "fail":
			if r, ok := results[key]; ok {
				r.passed = false
				r.output = failedOutputs[key]
			}
		}
	}

	cmd.Wait()

	// ── Print results grouped by package ─────────────────────────────────────

	fmt.Println()
	fmt.Println(strings.Repeat("=", 56))
	fmt.Println("  Job Matcher — Test Suite")
	fmt.Println(strings.Repeat("=", 56))

	totalPass := 0
	totalFail := 0
	var failures []string

	for _, pkg := range pkgOrder {
		// Short package name (last segment)
		parts := strings.Split(pkg, "/")
		shortPkg := parts[len(parts)-1]

		fmt.Printf("\n%s\n", shortPkg)

		// Collect tests for this package in sorted order
		var pkgResults []*testResult
		for _, r := range results {
			if r.pkg == pkg {
				pkgResults = append(pkgResults, r)
			}
		}
		sort.Slice(pkgResults, func(i, j int) bool {
			return pkgResults[i].name < pkgResults[j].name
		})

		for _, r := range pkgResults {
			if r.passed {
				fmt.Printf("  [✓] %s\n", r.name)
				totalPass++
			} else {
				fmt.Printf("  [X] %s\n", r.name)
				totalFail++
				key := r.pkg + "::" + r.name
				failures = append(failures, fmt.Sprintf("%s / %s", shortPkg, r.name))
				_ = key
				if len(r.output) > 0 {
					for _, line := range r.output {
						line = strings.TrimSpace(line)
						if line != "" {
							fmt.Printf("      %s\n", line)
						}
					}
				}
			}
		}
	}

	// ── Summary ───────────────────────────────────────────────────────────────

	fmt.Println()
	fmt.Println(strings.Repeat("-", 56))
	total := totalPass + totalFail
	fmt.Printf("  %d/%d passed", totalPass, total)
	if totalFail > 0 {
		fmt.Printf("  |  %d failed", totalFail)
	}
	fmt.Println()

	if len(failures) > 0 {
		fmt.Println()
		fmt.Println("FAILURES:")
		for _, f := range failures {
			fmt.Printf("  [X] %s\n", f)
		}
	}

	fmt.Println()

	if totalFail > 0 {
		os.Exit(1)
	}
}

// projectRoot walks up from the executable to find go.mod.
func projectRoot() string {
	// When run via "go run ./cmd/testrunner", working dir is the module root
	dir, _ := os.Getwd()
	return dir
}
