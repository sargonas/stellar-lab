package main

import (
	"io"
	"log"
	"os"
	"strings"
)

// ANSI escape codes used to style log output.
const (
	ansiReset  = "\033[0m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
)

// errLog writes errors, warnings, and fatal conditions to stderr. Normal,
// informational output goes to the standard log package, which we point at
// stdout in init below. This keeps container runtimes (Docker, etc.) from
// flagging routine operational logs as errors.
var errLog *log.Logger

func init() {
	// Route the default log package (log.Printf, ...) to stdout, and errors to
	// stderr, each wrapped so the timestamp is dimmed and errors are colorized.
	log.SetOutput(newColorWriter(os.Stdout, false))
	errLog = log.New(newColorWriter(os.Stderr, true), "", log.LstdFlags)
}

// colorWriter dims the timestamp of each log line and, on the error stream,
// tints the message red (errors) or yellow (warnings). Coloring is skipped when
// the destination is not a terminal, so piped output (Docker logs, files) and
// NO_COLOR environments stay clean.
type colorWriter struct {
	dst     io.Writer
	isError bool
	color   bool
}

func newColorWriter(f *os.File, isError bool) *colorWriter {
	return &colorWriter{dst: f, isError: isError, color: colorEnabled(f)}
}

// colorEnabled reports whether ANSI color should be emitted for f.
func colorEnabled(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func (c *colorWriter) Write(p []byte) (int, error) {
	if !c.color {
		return c.dst.Write(p)
	}

	line := strings.TrimRight(string(p), "\n")

	// Split the LstdFlags timestamp ("2006/01/02 15:04:05", 19 chars) from the
	// message so we can style them independently.
	ts, msg := line, ""
	if len(line) >= 19 {
		ts, msg = line[:19], line[19:]
	}

	out := ansiDim + ts + ansiReset
	if c.isError {
		msgColor := ansiRed
		if strings.Contains(msg, "Warning") || strings.Contains(msg, "WARNING") || strings.Contains(msg, "⚠") {
			msgColor = ansiYellow
		}
		out += msgColor + msg + ansiReset
	} else {
		out += msg
	}

	if _, err := io.WriteString(c.dst, out+"\n"); err != nil {
		return 0, err
	}
	return len(p), nil
}
