package logger

import (
	"context"
	"errors"
	"go/build"
	"log/slog"
	"os"
	"runtime"
	"strings"
)

var keyIgnore = "_logger_ignore"

func IgnoredAttr(val any) slog.Attr {
	return slog.Any(keyIgnore, val)
}

type HasLoggableAttrs interface {
	GetLoggableAttrs() []slog.Attr
}

type errWithAttr struct {
	err   error
	attrs []slog.Attr
}

func (e *errWithAttr) Error() string {
	return e.err.Error()
}

func (e *errWithAttr) GetLoggableAttrs() []slog.Attr {
	return e.attrs
}

func WithAttributes(err error, attrs ...slog.Attr) error {
	if err == nil {
		return nil
	}

	var ewa HasLoggableAttrs
	if errors.As(err, &ewa) {
		attrs = append(ewa.GetLoggableAttrs(), attrs...)
	}

	return &errWithAttr{
		err:   err,
		attrs: attrs,
	}
}

func getEnvOrDefault(key, default_ string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}

	return default_
}

var (
	logFormat = getEnvOrDefault("LOG_FORMAT", "json")
)

func SetupSLog(lvl slog.Level, rootPath string) {
	ho := slog.HandlerOptions{
		Level: lvl,
	}

	var h slog.Handler
	switch logFormat {
	case "json":
		h = slog.NewJSONHandler(os.Stderr, &ho)
		break
	case "text":
		h = slog.NewTextHandler(os.Stderr, &ho)
		break
	default:
		slog.Error("LOG_FORMAT must be json or text")
		os.Exit(1)
	}

	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = build.Default.GOPATH
	}

	slog.SetDefault(slog.New(&handler{
		baseHandler: h,
		rootPath:    strings.TrimSuffix(rootPath, "/") + "/",
		goPath:      strings.TrimSuffix(gopath, "/") + "/",
	}))
}

type handler struct {
	baseHandler slog.Handler
	rootPath    string
	goPath      string
}

func (e *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return e.baseHandler.Enabled(ctx, level)
}

func (e *handler) Handle(ctx context.Context, record slog.Record) error {
	newRecord := slog.Record{
		Time:    record.Time,
		Message: record.Message,
		Level:   record.Level,
		PC:      record.PC,
	}

	record.Attrs(func(attr slog.Attr) bool {
		ha, ok := attr.Value.Any().(HasLoggableAttrs)
		if !ok {
			var err error
			if err, ok = attr.Value.Any().(error); ok {
				ok = errors.As(err, &ha)
			}
		}

		if ok {
			newRecord.AddAttrs(ha.GetLoggableAttrs()...)
		}

		if attr.Key != keyIgnore {
			newRecord.AddAttrs(attr)
		}

		return true
	})

	record = newRecord

	fs := runtime.CallersFrames([]uintptr{record.PC})
	f, _ := fs.Next()
	file := f.File
	if strings.HasPrefix(file, e.rootPath) {
		file = file[len(e.rootPath):]
	} else if strings.HasPrefix(file, e.goPath) {
		file = file[len(e.goPath):]
	}
	record.AddAttrs(slog.Any(slog.SourceKey, &slog.Source{
		Function: f.Function,
		File:     file,
		Line:     f.Line,
	}))

	return e.baseHandler.Handle(ctx, record)
}

func (e *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &handler{
		baseHandler: e.baseHandler.WithAttrs(attrs),
		rootPath:    e.rootPath,
	}
}

func (e *handler) WithGroup(name string) slog.Handler {
	return &handler{
		baseHandler: e.baseHandler.WithGroup(name),
		rootPath:    e.rootPath,
	}
}
