// Package logx writes the §9 JSON event lines to ~/.ccctl/logs with rotation (A-019).
package logx

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// New returns a JSON logger writing to dir/ccctl.log (10 MB x 7 files) and to stderr.
func New(dir string) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	lj := &lumberjack.Logger{Filename: filepath.Join(dir, "ccctl.log"), MaxSize: 10, MaxBackups: 6}
	return slog.New(slog.NewJSONHandler(io.MultiWriter(lj, os.Stderr), nil)), lj, nil
}

// Event logs one §9 event line; the event name is both msg and the "event" key.
func Event(l *slog.Logger, event string, args ...any) {
	l.Info(event, append([]any{"event", event}, args...)...)
}
