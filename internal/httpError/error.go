package httpError

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"

	"transmission-proxy/internal/errHandler"
)

type HttpErrHandler struct {
	debugMode bool
}

func NewHttpErrHandler(debugMode bool) *HttpErrHandler {
	return &HttpErrHandler{
		debugMode: debugMode,
	}
}

func (h *HttpErrHandler) Handle(w http.ResponseWriter, ctx context.Context, tag int, status int, msg string, err error) {
	var errId string
	slogParams := []slog.Attr{slog.Any("error", err)}
	if !h.debugMode {
		errId = uuid.NewString()
		slogParams = append(slogParams, slog.String("err_id", errId))
	}

	l := slog.Default()

	if l.Enabled(ctx, slog.LevelError) {
		var pc uintptr
		var pcs [1]uintptr
		// skip [runtime.Callers, this function]
		runtime.Callers(2, pcs[:])
		pc = pcs[0]

		r := slog.NewRecord(time.Now(), slog.LevelError, msg, pc)
		r.AddAttrs(slogParams...)
		_ = l.Handler().Handle(ctx, r)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	data := map[string]any{}

	if tag != 0 {
		data["tag"] = tag
	}

	if h.debugMode {
		sb := strings.Builder{}
		sb.WriteString(msg)
		sb.WriteString(": ")
		sb.WriteString(err.Error())

		var ewa errHandler.ErrWithAttributes
		if errors.As(err, &ewa) {
			for _, a := range ewa.ErrorAttrs() {
				sb.WriteString(", ")
				sb.WriteString(a.Key)
				sb.WriteString(": ")
				sb.WriteString(a.Value.String())
			}
		}

		data["result"] = sb.String()
	} else {
		data["result"] = "Unknown error occurred: " + errId
	}

	bs, err := json.Marshal(data)
	if err != nil {
		slog.WarnContext(ctx, "cannot marshall error response body", slog.Any("error", err))
		bs = []byte("unknown error")
	}

	if _, err := fmt.Fprintln(w, string(bs)); err != nil {
		slog.WarnContext(ctx, "cannot write error response body", slog.Any("error", err))
	}
}
