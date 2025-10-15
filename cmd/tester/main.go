package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorBold   = "\033[1m"
)

type goTestEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

type pkgResult struct {
	name           string
	status         string
	elapsed        float64
	cached         bool
	noTestFiles    bool
	printed        bool
	packageOutputs []string
	testOutputs    map[string][]string
	failedTests    map[string]bool
	failureOrder   []string
}

type runner struct {
	packages  map[string]*pkgResult
	order     []*pkgResult
	passCount int
	failCount int
	skipCount int
	totalTime float64
	useColor  bool
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"./..."}
	}

	r := &runner{
		packages: make(map[string]*pkgResult),
		useColor: shouldUseColor(),
	}

	exitCode := r.run(args)
	os.Exit(exitCode)
}

func (r *runner) run(goArgs []string) int {
	cmdArgs := append([]string{"test", "-json"}, goArgs...)

	cmd := exec.Command("go", cmdArgs...)
	cmd.Env = os.Environ()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to read go test output: %v\n", err)
		return 1
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start go test: %v\n", err)
		return 1
	}

	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		r.processLine(line)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to read go test output: %v\n", err)
	}

	if err := cmd.Wait(); err != nil {
		r.printRemaining()
		r.printTotals()
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}

	r.printRemaining()
	r.printTotals()
	if r.failCount > 0 {
		return 1
	}
	return 0
}

func (r *runner) processLine(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}

	var event goTestEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		fmt.Println(line)
		return
	}

	if event.Package == "" {
		if event.Output != "" {
			fmt.Print(event.Output)
		}
		return
	}

	pkg := r.ensurePackage(event.Package)

	switch event.Action {
	case "run":
		// package started; nothing to do
	case "pass":
		if event.Test == "" {
			pkg.status = "pass"
			pkg.elapsed = event.Elapsed
		}
	case "fail":
		if event.Test == "" {
			pkg.status = "fail"
			pkg.elapsed = event.Elapsed
		} else {
			pkg.markTestFailed(event.Test)
		}
	case "skip":
		if event.Test == "" {
			pkg.status = "skip"
			pkg.noTestFiles = true
		}
	case "output":
		r.handleOutput(pkg, event)
	}
}

func (r *runner) ensurePackage(name string) *pkgResult {
	if pkg, ok := r.packages[name]; ok {
		return pkg
	}

	pkg := &pkgResult{
		name:        name,
		testOutputs: make(map[string][]string),
		failedTests: make(map[string]bool),
	}
	r.packages[name] = pkg
	r.order = append(r.order, pkg)
	return pkg
}

func (r *runner) handleOutput(pkg *pkgResult, event goTestEvent) {
	line := trimTrailingNewline(event.Output)
	if strings.TrimSpace(line) == "" {
		return
	}

	if event.Test != "" {
		pkg.testOutputs[event.Test] = append(pkg.testOutputs[event.Test], line)
		return
	}

	switch {
	case strings.HasPrefix(line, "ok"):
		if strings.Contains(line, "(cached)") {
			pkg.cached = true
		}
		if pkg.status == "" {
			pkg.status = "pass"
		}
		r.printSummary(pkg)
	case strings.HasPrefix(line, "FAIL"):
		if pkg.status == "" {
			pkg.status = "fail"
		}
		r.printSummary(pkg)
	case strings.HasPrefix(line, "?"):
		pkg.status = "skip"
		pkg.noTestFiles = true
		r.printSummary(pkg)
	default:
		pkg.packageOutputs = append(pkg.packageOutputs, line)
	}
}

func (pkg *pkgResult) markTestFailed(name string) {
	if pkg.failedTests[name] {
		return
	}
	pkg.failedTests[name] = true
	pkg.failureOrder = append(pkg.failureOrder, name)
}

func (r *runner) printSummary(pkg *pkgResult) {
	if pkg.printed {
		return
	}

	switch pkg.status {
	case "pass":
		r.passCount++
		r.totalTime += pkg.elapsed
		fmt.Printf("%s %s %s%s\n",
			r.paint(colorGreen, "✓"),
			r.paint(colorBold+colorGreen, "PASS"),
			pkg.name,
			pkg.summarySuffix(),
		)
	case "fail":
		r.failCount++
		r.totalTime += pkg.elapsed
		fmt.Printf("%s %s %s%s\n",
			r.paint(colorRed, "✗"),
			r.paint(colorBold+colorRed, "FAIL"),
			pkg.name,
			pkg.summarySuffix(),
		)
		for _, line := range pkg.packageOutputs {
			fmt.Printf("    %s\n", line)
		}
		for _, test := range pkg.failureOrder {
			fmt.Printf("    %s %s\n", r.paint(colorRed, "✗"), test)
			for _, out := range pkg.testOutputs[test] {
				fmt.Printf("        %s\n", out)
			}
		}
	case "skip":
		r.skipCount++
		fmt.Printf("%s %s %s%s\n",
			r.paint(colorYellow, "•"),
			r.paint(colorBold+colorYellow, "SKIP"),
			pkg.name,
			pkg.summarySuffix(),
		)
	default:
		// Unknown status - print raw outputs
		for _, line := range pkg.packageOutputs {
			fmt.Println(line)
		}
	}

	pkg.printed = true
}

func (pkg *pkgResult) summarySuffix() string {
	var parts []string
	if pkg.elapsed > 0 {
		parts = append(parts, fmt.Sprintf("%.3fs", pkg.elapsed))
	}
	if pkg.cached {
		parts = append(parts, "cached")
	}
	if pkg.noTestFiles {
		parts = append(parts, "no tests")
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func (r *runner) printRemaining() {
	for _, pkg := range r.order {
		if !pkg.printed && pkg.status != "" {
			r.printSummary(pkg)
		}
	}
}

func (r *runner) printTotals() {
	fmt.Println()
	fmt.Printf("%s %d passed\n", r.paint(colorGreen, "✓"), r.passCount)
	if r.failCount > 0 {
		fmt.Printf("%s %d failed\n", r.paint(colorRed, "✗"), r.failCount)
	}
	if r.skipCount > 0 {
		fmt.Printf("%s %d skipped\n", r.paint(colorYellow, "•"), r.skipCount)
	}
	if r.totalTime > 0 {
		fmt.Printf("Σ %.3fs total\n", r.totalTime)
	}
}

func (r *runner) paint(color string, text string) string {
	if !r.useColor {
		return text
	}
	return color + text + colorReset
}

func trimTrailingNewline(s string) string {
	s = strings.TrimRight(s, "\r\n")
	return s
}

func shouldUseColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
