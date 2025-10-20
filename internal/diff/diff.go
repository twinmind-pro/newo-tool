package diff

import (
	"fmt"
	"strings"
	"unicode/utf8"
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

	firstLocal, firstRemote := headerLineNumbers(lines)
	header := fmt.Sprintf("diff %s (@@ -%d +%d @@)", path, firstLocal, firstRemote)

	type row struct {
		displayNumber string
		plainNumber   string
		displayText   string
		plainText     string
	}

	rows := make([]row, 0, len(lines))
	for _, line := range lines {
		displayNumber, plainNumber := formatLineNumber(line)
		displayText, plainText := formatLineText(line)
		rows = append(rows, row{
			displayNumber: displayNumber,
			plainNumber:   plainNumber,
			displayText:   displayText,
			plainText:     plainText,
		})
	}

	col1Width := 0
	col2Width := 0
	for _, r := range rows {
		if l := visibleLength(r.plainNumber); l > col1Width {
			col1Width = l
		}
		if l := visibleLength(r.plainText); l > col2Width {
			col2Width = l
		}
	}
	if col1Width < 2 {
		col1Width = 2
	}

	tableWidth := col1Width + col2Width + 7
	headerInnerWidth := tableWidth - 4
	if headerLen := visibleLength(header); headerLen > headerInnerWidth {
		col2Width += headerLen - headerInnerWidth
		tableWidth = col1Width + col2Width + 7
		headerInnerWidth = tableWidth - 4
	}

	var builder strings.Builder
	topBorder := buildBorderLine(col1Width, col2Width)
	builder.WriteString(topBorder)
	builder.WriteString(buildHeaderLine(header, headerInnerWidth))
	builder.WriteString(topBorder)
	for _, r := range rows {
		builder.WriteString(buildDataLine(r.displayNumber, r.plainNumber, col1Width, true, r.displayText, r.plainText, col2Width))
	}
	builder.WriteString(topBorder)
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

func headerLineNumbers(lines []Line) (int, int) {
	firstLocal, firstRemote := 0, 0
	for _, line := range lines {
		switch line.Kind {
		case "context":
			if firstLocal == 0 && line.LocalLine > 0 {
				firstLocal = line.LocalLine
			}
			if firstRemote == 0 && line.RemoteLine > 0 {
				firstRemote = line.RemoteLine
			}
		case "del":
			if firstLocal == 0 && line.LocalLine > 0 {
				firstLocal = line.LocalLine
			}
		case "add":
			if firstRemote == 0 && line.RemoteLine > 0 {
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
	return firstLocal, firstRemote
}

func formatLineNumber(line Line) (display string, plain string) {
	switch line.Kind {
	case "del":
		plain = fmt.Sprintf("-%d", line.LocalLine)
		display = fmt.Sprintf("%s-%d%s", redColor, line.LocalLine, resetColor)
	case "add":
		plain = fmt.Sprintf("+%d", line.RemoteLine)
		display = fmt.Sprintf("%s+%d%s", greenColor, line.RemoteLine, resetColor)
	default:
		n := line.LocalLine
		if n == 0 {
			n = line.RemoteLine
		}
		plain = fmt.Sprintf("%d", n)
		display = plain
	}
	return display, plain
}

func formatLineText(line Line) (display string, plain string) {
	switch line.Kind {
	case "del":
		return fmt.Sprintf("%s%s%s", redColor, line.Text, resetColor), line.Text
	case "add":
		return fmt.Sprintf("%s%s%s", greenColor, line.Text, resetColor), line.Text
	default:
		return line.Text, line.Text
	}
}

func buildBorderLine(col1Width, col2Width int) string {
	var builder strings.Builder
	builder.WriteString("  +")
	builder.WriteString(strings.Repeat("-", col1Width+2))
	builder.WriteString("+")
	builder.WriteString(strings.Repeat("-", col2Width+2))
	builder.WriteString("+\n")
	return builder.String()
}

func buildHeaderLine(header string, innerWidth int) string {
	padding := innerWidth - visibleLength(header)
	if padding < 0 {
		padding = 0
	}
	leftPad := padding / 2
	rightPad := padding - leftPad
	var builder strings.Builder
	builder.WriteString("  | ")
	builder.WriteString(strings.Repeat(" ", leftPad))
	builder.WriteString(header)
	builder.WriteString(strings.Repeat(" ", rightPad))
	builder.WriteString(" |\n")
	return builder.String()
}

func buildDataLine(displayNumber, plainNumber string, widthNumber int, alignRight bool, displayText, plainText string, widthText int) string {
	var builder strings.Builder
	builder.WriteString("  | ")
	builder.WriteString(padANSI(displayNumber, plainNumber, widthNumber, alignRight))
	builder.WriteString(" | ")
	builder.WriteString(padANSI(displayText, plainText, widthText, false))
	builder.WriteString(" |\n")
	return builder.String()
}

func padANSI(display, plain string, width int, alignRight bool) string {
	pad := width - visibleLength(plain)
	if pad < 0 {
		pad = 0
	}
	if alignRight {
		return strings.Repeat(" ", pad) + display
	}
	return display + strings.Repeat(" ", pad)
}

func visibleLength(s string) int {
	return utf8.RuneCountInString(s)
}
