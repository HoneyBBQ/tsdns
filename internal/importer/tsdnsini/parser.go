// Package tsdnsini provides a parser for the official TeamSpeak TSDNS configuration files.
package tsdnsini

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var errReaderNil = errors.New("reader is nil")

// Entry represents a single "ident=value" pair parsed from a TSDNS ini file.
type Entry struct {
	Ident string
	Value string
	Line  int
}

// ParseResult contains the entries successfully parsed and the number of skipped lines.
type ParseResult struct {
	Entries []Entry
	Skipped int
}

// ParseFile parses an official TeamSpeak tsdns_settings.ini file from the given file path.
func ParseFile(path string) (ParseResult, error) {
	// Clean path to mitigate potential G304 (File Inclusion).
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return ParseResult{}, err
	}
	defer func() { _ = f.Close() }()

	return Parse(f)
}

// Parse parses "ident=value" entries from an io.Reader.
// Lines starting with "#" are ignored.
func Parse(r io.Reader) (ParseResult, error) {
	if r == nil {
		return ParseResult{}, errReaderNil
	}

	var res ParseResult

	sc := bufio.NewScanner(r)
	// Value lines can be long when multiple IPs are configured.
	const (
		initialBufSize = 64 * 1024
		maxBufSize     = 1024 * 1024
	)
	sc.Buffer(make([]byte, 0, initialBufSize), maxBufSize)

	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		const expectedParts = 2
		parts := strings.SplitN(line, "=", expectedParts)
		if len(parts) != expectedParts {
			res.Skipped++

			continue
		}

		ident := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if ident == "" || value == "" {
			res.Skipped++

			continue
		}

		res.Entries = append(res.Entries, Entry{
			Ident: ident,
			Value: value,
			Line:  lineNo,
		})
	}
	err := sc.Err()
	if err != nil {
		return ParseResult{}, fmt.Errorf("scan: %w", err)
	}

	return res, nil
}
