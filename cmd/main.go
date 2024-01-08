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

	"transmission-proxy/internal/jrpc"
	"transmission-proxy/internal/logger"
	"transmission-proxy/internal/response"
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

	debugMode = getBoolEnv("DEBUG_MODE")
)

type rpcTag struct{}

func proxy(gw *url.URL, rr *response.Responder) http.HandlerFunc {
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

			rr.RespondAndLogCustom(w, r.Context(), fmt.Errorf("upstream error: %w", err), tag, slog.LevelError, http.StatusBadGateway)
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
			slog.ErrorContext(r.Context(), "proxy: failed to write response: "+err.Error(), logger.IgnoredAttr(err))
		}
	}
}

func rpcProxy(gw http.Handler, v transmission.RequestValidator, rr *response.Responder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := jrpc.FromRequest(r)
		if err != nil {
			rr.RespondAndLogCustom(w, r.Context(), fmt.Errorf("failed to unmarshal RPC request: %w", err), 0, slog.LevelError, http.StatusBadRequest)
			return
		}

		if err = v.Validate(req); err != nil {
			rr.RespondAndLogCustom(w, r.Context(), fmt.Errorf("invalid RPC request: %w", err), req.Tag, slog.LevelError, http.StatusBadRequest)
			return
		}

		bs, err := json.Marshal(req)
		if err != nil {
			rr.RespondAndLogError(w, r.Context(), fmt.Errorf("cannot serialize RPC request: %w", err), req.Tag)
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
			slog.ErrorContext(r.Context(), "not_found: failed to write response: "+err.Error(), logger.IgnoredAttr(err))
		}
	}
}

func main() {
	_, thisFile, _, _ := runtime.Caller(0)
	logger.SetupSLog(slog.LevelDebug, path.Dir(path.Dir(thisFile)))

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
	if !strings.HasSuffix(upstreamHost, "/") {
		upstreamHost += "/"
	}
	gw, err := url.Parse(upstreamHost)
	if err != nil {
		slog.Error("failed to parse UPSTREAM_HOST: "+err.Error(), logger.IgnoredAttr(err))
		os.Exit(1)
	}
	if gw.Path != "/" || gw.RawQuery != "" || gw.Fragment != "" {
		slog.Error("UPSTREAM_HOST must not define path or query")
		os.Exit(1)
	}

	v := transmission.DefaultMethodsValidator(downloadPrefix)

	rr := &response.Responder{DebugMode: debugMode}

	p := proxy(gw, rr)
	http.Handle(webPath, p)
	http.Handle(rpcPath, rpcProxy(p, v, rr))
	http.Handle("/", homePage(p))

	err = http.ListenAndServe(":8080", nil)

	slog.Error("aborting: "+err.Error(), logger.IgnoredAttr(err))
	os.Exit(1)
}
