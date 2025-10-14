package diff

import (
	"fmt"
	"strings"
)

const (
	redColor   = "\033[31m"
	greenColor = "\033[32m"
	resetColor = "\033[0m"
)

// Line represents a single line in a diff output.
type Line struct {
	Kind       string
	Text       string
	LocalLine  int
	RemoteLine int
}

// Generate computes the diff between two byte slices and returns a slice of Lines.
// The context parameter controls how many lines of surrounding context to include.
func Generate(local, remote []byte, context int) []Line {
	if looksBinary(local) || looksBinary(remote) {
		return nil
	}
	full := fullLines(local, remote)
	if context < 0 {
		return full
	}
	return trimContext(full, context)
}

// Format takes a slice of diff Lines and formats them into a human-readable, colored string with headers.
func Format(path string, lines []Line) string {
	if len(lines) == 0 {
		return ""
	}

	var builder strings.Builder

	firstLocal, firstRemote := 0, 0
	for _, line := range lines {
		switch line.Kind {
		case "context":
			if firstLocal == 0 {
				firstLocal = line.LocalLine
			}
			if firstRemote == 0 {
				firstRemote = line.RemoteLine
			}
		case "del":
			if firstLocal == 0 {
				firstLocal = line.LocalLine
			}
		case "add":
			if firstRemote == 0 {
				firstRemote = line.RemoteLine
			}
		}
		if firstLocal != 0 && firstRemote != 0 {
			break
		}
	}
	if firstLocal == 0 {
		firstLocal = 1
	}
	if firstRemote == 0 {
		firstRemote = 1
	}

	builder.WriteString(fmt.Sprintf("  diff %s (@@ -%d +%d @@):\n", path, firstLocal, firstRemote))
	for _, line := range lines {
		switch line.Kind {
		case "context":
			builder.WriteString(fmt.Sprintf("    %4d | %s\n", line.LocalLine, line.Text))
		case "del":
			builder.WriteString(fmt.Sprintf("  %s-%4d | %s%s\n", redColor, line.LocalLine, line.Text, resetColor))
		case "add":
			builder.WriteString(fmt.Sprintf("  %s+%4d | %s%s\n", greenColor, line.RemoteLine, line.Text, resetColor))
		}
	}
	return builder.String()
}

func fullLines(local, remote []byte) []Line {
	localLines := splitLines(local)
	remoteLines := splitLines(remote)

	m, n := len(localLines), len(remoteLines)
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}

	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if localLines[i] == remoteLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var diff []Line
	i, j := 0, 0
	for i < m || j < n {
		switch {
		case i < m && j < n && localLines[i] == remoteLines[j]:
			diff = append(diff, Line{
				Kind:       "context",
				Text:       localLines[i],
				LocalLine:  i + 1,
				RemoteLine: j + 1,
			})
			i++
			j++
		case j < n && (i == m || lcs[i][j+1] >= lcs[i+1][j]):
			diff = append(diff, Line{
				Kind:       "add",
				Text:       remoteLines[j],
				RemoteLine: j + 1,
			})
			j++
		case i < m:
			diff = append(diff, Line{
				Kind:      "del",
				Text:      localLines[i],
				LocalLine: i + 1,
			})
			i++
		default:
			j++
		}
	}

	return diff
}

func trimContext(lines []Line, context int) []Line {
	if len(lines) == 0 {
		return lines
	}
	keep := make([]bool, len(lines))
	hasChange := false
	for idx, line := range lines {
		if line.Kind != "context" {
			hasChange = true
			start := idx - context
			if start < 0 {
				start = 0
			}
			end := idx + context + 1
			if end > len(lines) {
				end = len(lines)
			}
			for k := start; k < end; k++ {
				keep[k] = true
			}
		}
	}
	if !hasChange {
		return nil
	}
	result := make([]Line, 0, len(lines))
	for idx, line := range lines {
		if keep[idx] {
			result = append(result, line)
		}
	}
	return result
}

func splitLines(content []byte) []string {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func looksBinary(data []byte) bool {
	const sample = 200
	for i, b := range data {
		if i >= sample {
			break
		}
		if b == 0 {
			return true
		}
	}
	return false
}
