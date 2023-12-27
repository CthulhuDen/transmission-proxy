package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"

	_ "github.com/joho/godotenv/autoload"

	"transmission-proxy/internal/errHandler"
	"transmission-proxy/internal/httpError"
	"transmission-proxy/internal/jrpc"
	"transmission-proxy/internal/transmission"
)

func getEnvOrDefault(key, default_ string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}

	return default_
}

func getBoolEnv(key string) bool {
	if val := strings.ToLower(os.Getenv(key)); val == "yes" || val == "on" || val == "true" {
		return true
	}

	return false
}

var (
	downloadPrefix = os.Getenv("DOWNLOAD_PREFIX")
	upstreamHost   = os.Getenv("UPSTREAM_HOST")
	webPath        = getEnvOrDefault("WEB_PATH", "/transmission/web/")
	rpcPath        = getEnvOrDefault("RPC_PATH", "/transmission/rpc")

	logFormat = getEnvOrDefault("LOG_FORMAT", "json")
	debugMode = getBoolEnv("DEBUG_MODE")
)

type rpcTag struct{}

func proxy(gw *url.URL, eh *httpError.HttpErrHandler) http.HandlerFunc {
	c := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		u := gw.JoinPath(r.URL.Path)
		u.RawQuery = r.URL.RawQuery
		r.URL = u
		r.RequestURI = ""

		resp, err := c.Do(r)
		if err != nil {
			var tag int
			if t := r.Context().Value(rpcTag{}); t != nil {
				tag = t.(int)
			}

			eh.Handle(w, r.Context(), tag, http.StatusBadGateway, "upstream error", err)
			return
		}

		for h, vals := range resp.Header {
			for _, val := range vals {
				w.Header().Add(h, val)
			}
		}

		w.WriteHeader(resp.StatusCode)

		defer func() { _ = resp.Body.Close() }()

		_, err = io.Copy(w, resp.Body)
		if err != nil {
			slog.ErrorContext(r.Context(), "proxy: failed to write response", slog.Any("error", err))
		}
	}
}

func rpcProxy(gw http.Handler, v transmission.RequestValidator, eh *httpError.HttpErrHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := jrpc.FromRequest(r)
		if err != nil {
			eh.Handle(w, r.Context(), 0, http.StatusBadRequest, "cannot unmarshal RPC request", err)
			return
		}

		if err = v.Validate(req); err != nil {
			eh.Handle(w, r.Context(), req.Tag, http.StatusBadRequest, "invalid RPC request", err)
			return
		}

		bs, err := json.Marshal(req)
		if err != nil {
			eh.Handle(w, r.Context(), req.Tag, http.StatusInternalServerError, "cannot serialize RPC request", err)
			return
		}

		r.ContentLength = -1
		r.Header.Del("Content-Length")
		r.Body = io.NopCloser(bytes.NewReader(bs))

		gw.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), rpcTag{}, req.Tag)))
	}
}

func homePage(gw http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			gw.ServeHTTP(w, r)
			return
		}

		data := map[string]any{}
		data["result"] = "page not found"

		bs, _ := json.Marshal(data)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)

		if _, err := fmt.Fprintln(w, string(bs)); err != nil {
			slog.ErrorContext(r.Context(), "not_found: failed to write response", slog.Any("error", err))
		}
	}
}

func main() {
	_, errHandler.RootPath, _, _ = runtime.Caller(0)
	errHandler.RootPath = path.Dir(path.Dir(errHandler.RootPath)) + "/"
	setupLogger()

	if downloadPrefix == "" {
		slog.Error("DOWNLOAD_PREFIX must be defined")
		os.Exit(1)
	}
	if downloadPrefix[0] != '/' {
		slog.Error("DOWNLOAD_PREFIX must begin with /")
		os.Exit(1)
	}
	if downloadPrefix[len(downloadPrefix)-1] != '/' {
		slog.Error("DOWNLOAD_PREFIX must end with /")
		os.Exit(1)
	}

	if upstreamHost == "" {
		slog.Error("UPSTREAM_HOST must be defined")
		os.Exit(1)
	}
	gw, err := url.Parse(upstreamHost)
	if err != nil {
		slog.Error("failed to parse UPSTREAM_HOST", slog.Any("error", err))
		os.Exit(1)
	}
	if gw.Path != "" || gw.RawQuery != "" {
		slog.Error("UPSTREAM_HOST must not define path or query")
		os.Exit(1)
	}

	v := transmission.DefaultMethodsValidator(downloadPrefix)

	eh := httpError.NewHttpErrHandler(debugMode)

	p := proxy(gw, eh)
	http.Handle(webPath, p)
	http.Handle(rpcPath, rpcProxy(p, v, eh))
	http.Handle("/", homePage(p))

	slog.Error("aborting", slog.Any("error", http.ListenAndServe(":8080", nil)))
	os.Exit(1)
}

func setupLogger() {
	ho := slog.HandlerOptions{
		Level: slog.LevelInfo,
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

	slog.SetDefault(slog.New(&errHandler.ErrHandler{BaseHandler: h}))
}
