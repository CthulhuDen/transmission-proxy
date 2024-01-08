package response

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"transmission-proxy/internal/logger"
)

type Responder struct {
	DebugMode bool
}

func (rr *Responder) RespondAndLogError(w http.ResponseWriter, ctx context.Context, err error, tag int) {
	errId := rr.renderErrorReturnID(w, ctx, http.StatusInternalServerError, err.Error(), tag)
	log(ctx, slog.LevelError, err.Error(), errId, logger.IgnoredAttr(err))
}

func (rr *Responder) RespondAndLogCustom(w http.ResponseWriter, ctx context.Context, err error, tag int, lvl slog.Level, status int) {
	errId := rr.renderErrorReturnID(w, ctx, status, err.Error(), tag)
	log(ctx, lvl, err.Error(), errId, logger.IgnoredAttr(err))
}

func (rr *Responder) renderErrorReturnID(w http.ResponseWriter, ctx context.Context, status int, message string, tag int) slog.Attr {
	data := map[string]any{}

	if tag != 0 {
		data["tag"] = tag
	}

	errId := uuid.NewString()

	if rr.DebugMode {
		r, s := utf8.DecodeRuneInString(message)
		data["result"] = string(unicode.ToUpper(r)) + message[s:]
	} else {
		data["result"] = "Unknown error occurred while processing your request. Error ID: " + errId
	}

	bs, err := json.Marshal(data)
	if err == nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	} else {
		slog.ErrorContext(ctx, "cannot marshall error response body: "+err.Error(), logger.IgnoredAttr(err))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		bs = []byte("unknown error")
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = io.Copy(w, bytes.NewReader(bs))

	return slog.String("err_id", errId)
}

func log(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	l := slog.Default()

	if !l.Enabled(ctx, level) {
		return
	}

	var pc uintptr
	var pcs [1]uintptr
	// skip [runtime.Callers, this function, this function's caller]
	runtime.Callers(3, pcs[:])
	pc = pcs[0]

	r := slog.NewRecord(time.Now(), level, msg, pc)
	r.AddAttrs(attrs...)
	_ = l.Handler().Handle(ctx, r)
}
