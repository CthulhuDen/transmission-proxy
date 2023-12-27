package errHandler

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"strings"
)

var RootPath string

type ErrWithAttributes interface {
	ErrorAttrs() []slog.Attr
}

type errWithAttr struct {
	err   error
	attrs []slog.Attr
}

func (e *errWithAttr) Error() string {
	return e.err.Error()
}

func (e *errWithAttr) ErrorAttrs() []slog.Attr {
	return e.attrs
}

func WithAttributes(err error, attrs ...slog.Attr) error {
	if err == nil {
		return nil
	}

	var ewa ErrWithAttributes
	if errors.As(err, &ewa) {
		attrs = append(ewa.ErrorAttrs(), attrs...)
	}

	return &errWithAttr{
		err:   err,
		attrs: attrs,
	}
}

type ErrHandler struct {
	BaseHandler slog.Handler
}

func (e *ErrHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return e.BaseHandler.Enabled(ctx, level)
}

func (e *ErrHandler) Handle(ctx context.Context, record slog.Record) error {
	record = record.Clone()
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key != "error" {
			return true
		}

		if err, ok := attr.Value.Any().(error); ok {
			var ewa ErrWithAttributes
			if errors.As(err, &ewa) {
				record.AddAttrs(ewa.ErrorAttrs()...)
			}
		}

		return false
	})

	fs := runtime.CallersFrames([]uintptr{record.PC})
	f, _ := fs.Next()
	file := f.File
	if strings.HasPrefix(file, RootPath) {
		file = file[len(RootPath):]
	}
	record.AddAttrs(slog.Any(slog.SourceKey, &slog.Source{
		Function: f.Function,
		File:     file,
		Line:     f.Line,
	}))

	return e.BaseHandler.Handle(ctx, record)
}

func (e *ErrHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ErrHandler{e.BaseHandler.WithAttrs(attrs)}
}

func (e *ErrHandler) WithGroup(name string) slog.Handler {
	return &ErrHandler{e.BaseHandler.WithGroup(name)}
}
