package console

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiBlue   = "\033[34m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiGray   = "\033[90m"
)

// Writer orchestrates styled console output to stdout/stderr.
type Writer struct {
	out   io.Writer
	err   io.Writer
	theme theme

	mu    sync.Mutex
	wrote bool
}

// Option customises writer behaviour.
type Option func(*options)

type options struct {
	colorOverride *bool
}

// WithColors forces colour output (true enables, false disables) regardless of terminal detection.
func WithColors(enabled bool) Option {
	return func(o *options) {
		o.colorOverride = ptr(enabled)
	}
}

func ptr[T any](v T) *T {
	return &v
}

// New constructs a console writer. Colours are enabled when stdout looks like a TTY,
// the NO_COLOR convention is not set, and no override is provided.
func New(out, err io.Writer, opts ...Option) *Writer {
	if out == nil {
		out = io.Discard
	}
	if err == nil {
		err = io.Discard
	}

	cfg := options{}
	for _, opt := range opts {
		opt(&cfg)
	}

	var enabled bool
	noColor := hasNoColor()
	switch {
	case noColor:
		enabled = false
	case cfg.colorOverride != nil:
		enabled = *cfg.colorOverride
	default:
		enabled = detectTTY(out)
	}

	return &Writer{
		out: out,
		err: err,
		theme: theme{
			colorEnabled: enabled,
		},
	}
}

func hasNoColor() bool {
	_, exists := os.LookupEnv("NO_COLOR")
	return exists
}

func detectTTY(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// Section prints a highlighted section heading.
func (w *Writer) Section(title string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.wrote {
		_, _ = fmt.Fprintln(w.out)
	}

	headline := fmt.Sprintf("== %s ==", strings.TrimSpace(title))
	headline = w.theme.style(headline, ansiBold, ansiBlue)
	_, _ = fmt.Fprintln(w.out, headline)
	w.wrote = true
}

// Info prints a neutral informational line.
func (w *Writer) Info(format string, args ...any) {
	w.printLine(w.out, "[i]", ansiBlue, nil, format, args...)
}

// Success prints a success line.
func (w *Writer) Success(format string, args ...any) {
	w.printLine(w.out, "[+]", ansiGreen, []string{ansiBold}, format, args...)
}

// Warn prints a warning line to stderr.
func (w *Writer) Warn(format string, args ...any) {
	w.printLine(w.err, "[!]", ansiYellow, nil, format, args...)
}

// Error prints an error line to stderr.
func (w *Writer) Error(format string, args ...any) {
	w.printLine(w.err, "[x]", ansiRed, []string{ansiBold}, format, args...)
}

// List prints a bulleted list to stdout.
func (w *Writer) List(items []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(items) == 0 {
		return
	}
	bullet := w.theme.style("-", ansiGray)
	for _, item := range items {
		_, _ = fmt.Fprintf(w.out, "    %s %s\n", bullet, strings.TrimSpace(item))
	}
	w.wrote = true
}

// RawLine prints a line verbatim to stdout.
func (w *Writer) RawLine(format string, args ...any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = fmt.Fprintf(w.out, format+"\n", args...)
	w.wrote = true
}

// Write emits text to stdout without forcing a trailing newline.
func (w *Writer) Write(text string) {
	if text == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = fmt.Fprint(w.out, text)
	w.wrote = true
}

// WriteErr emits text to stderr without forcing a newline.
func (w *Writer) WriteErr(text string) {
	if text == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = fmt.Fprint(w.err, text)
	w.wrote = true
}

// Prompt writes a prompt without a newline, intended for interactive questions.
func (w *Writer) Prompt(format string, args ...any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = fmt.Fprintf(w.out, format, args...)
	w.wrote = true
}

func (w *Writer) printLine(target io.Writer, icon, iconColor string, msgStyles []string, format string, args ...any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	styledIcon := w.theme.style(icon, iconColor, ansiBold)
	styledMsg := w.theme.style(msg, msgStyles...)
	_, _ = fmt.Fprintf(target, "  %s %s\n", styledIcon, styledMsg)
	w.wrote = true
}

type theme struct {
	colorEnabled bool
}

func (t theme) style(text string, codes ...string) string {
	if !t.colorEnabled || len(codes) == 0 {
		return text
	}
	var b strings.Builder
	for _, code := range codes {
		if code != "" {
			b.WriteString(code)
		}
	}
	b.WriteString(text)
	b.WriteString(ansiReset)
	return b.String()
}
