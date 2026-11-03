package slogpretty

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
)

type PrettyHandler struct {
	opts slog.HandlerOptions
	out  io.Writer
	l    *slog.Logger
}

type PrettyHandlerOptions struct {
	SlogOpts slog.HandlerOptions
}

func (opts PrettyHandlerOptions) NewPrettyHandler(out io.Writer) *PrettyHandler {
	h := &PrettyHandler{
		opts: opts.SlogOpts,
		out:  out,
	}

	return h
}

func (h *PrettyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *PrettyHandler) Handle(ctx context.Context, r slog.Record) error {
	level := r.Level.String() + ":"

	switch r.Level {
	case slog.LevelDebug:
		level = "\033[34m" + level + "\033[0m"
	case slog.LevelInfo:
		level = "\033[32m" + level + "\033[0m"
	case slog.LevelWarn:
		level = "\033[33m" + level + "\033[0m"
	case slog.LevelError:
		level = "\033[31m" + level + "\033[0m"
	}

	timeStr := r.Time.Format("[15:04:05.000]")
	msg := "\033[37m" + r.Message + "\033[0m"

	fields := make(map[string]any, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.Any()
		return true
	})

	var b []byte
	if len(fields) > 0 {
		var err error
		b, err = json.MarshalIndent(fields, "", "  ")
		if err != nil {
			return err
		}
	}

	res := timeStr + " " + level + " " + msg
	if len(b) > 0 {
		res += "\n" + string(b)
	}
	res += "\n"

	_, err := h.out.Write([]byte(res))
	return err
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	return h
}
