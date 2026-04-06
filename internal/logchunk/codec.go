package logchunk

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	StreamStdout = "stdout"
	StreamStderr = "stderr"
)

const maxFrameLineBytes = 256 * 1024

// Record is the framed log line contract stored inside gzipped log chunks.
type Record struct {
	Stream string `json:"stream"`
	Line   string `json:"line"`
}

func normalizeStream(stream string) string {
	switch strings.ToLower(strings.TrimSpace(stream)) {
	case StreamStderr:
		return StreamStderr
	default:
		return StreamStdout
	}
}

// EncodeRecordLine marshals one log frame as JSON and appends a '\n' delimiter.
func EncodeRecordLine(w io.Writer, stream, line string) error {
	rec := Record{
		Stream: normalizeStream(stream),
		Line:   line,
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal log record: %w", err)
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n")); err != nil {
		return err
	}
	return nil
}

// DecodeGzip parses stream-aware framed logs from gzipped bytes.
func DecodeGzip(data []byte) ([]Record, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()

	var records []Record
	scanner := bufio.NewScanner(zr)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxFrameLineBytes)
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			return nil, fmt.Errorf("decode log frame: %w", err)
		}
		rec.Stream = normalizeStream(rec.Stream)
		rec.Line = strings.TrimRight(rec.Line, "\r")
		if rec.Line == "" {
			continue
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("log chunk contains no decodable records")
	}
	return records, nil
}
