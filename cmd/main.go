package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	_ "github.com/joho/godotenv/autoload"

	http_error "transmission-proxy/internal/http-error"
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

	debugMode = getBoolEnv("DEBUG_MODE")
)

type rpcTag struct{}

func proxy(gw *url.URL, eh *http_error.HttpErrHandler) http.HandlerFunc {
	c := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		u := *gw
		u.Path = r.URL.Path
		u.RawQuery = r.URL.RawQuery

		r.URL = &u
		r.RequestURI = ""

		resp, err := c.Do(r)
		if err != nil {
			var tag int
			if t := r.Context().Value(rpcTag{}); t != nil {
				tag = t.(int)
			}

			eh.Handle(w, tag, http.StatusBadGateway, "upstream error:", err)
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
			log.Println("proxy: failed writing response: ", err)
		}
	}
}

func rpcProxy(gw http.Handler, v transmission.RequestValidator, eh *http_error.HttpErrHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := jrpc.FromRequest(r)
		if err != nil {
			eh.Handle(w, 0, http.StatusBadRequest, "unmarshal RPC request:", err)
			return
		}

		if err = v.Validate(req); err != nil {
			eh.Handle(w, req.Tag, http.StatusBadRequest, "invalid RPC request:", err)
			return
		}

		bs, err := json.Marshal(req)
		if err != nil {
			eh.Handle(w, req.Tag, http.StatusInternalServerError, "serializing RPC request:", err)
			return
		}

		r.ContentLength = -1
		r.Header.Del("Content-Length")
		r.Body = io.NopCloser(bytes.NewReader(bs))

		gw.ServeHTTP(w, r)
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
			log.Println("while writing error body:", err)
		}
	}
}

func main() {
	if downloadPrefix == "" {
		log.Fatalln("DOWNLOAD_PREFIX must be defined")
	}
	if downloadPrefix[0] != '/' {
		log.Fatalln("DOWNLOAD_PREFIX must begin with /")
	}
	if downloadPrefix[len(downloadPrefix)-1] != '/' {
		log.Fatalln("DOWNLOAD_PREFIX must end with /")
	}

	if upstreamHost == "" {
		log.Fatalln("UPSTREAM_HOST must be defined")
	}
	gw, err := url.Parse(upstreamHost)
	if err != nil {
		log.Fatalln("failed to parse UPSTREAM_HOST:", err)
	}
	if gw.Path != "" || gw.RawQuery != "" {
		log.Fatalln("UPSTREAM_HOST must not define path or query")
	}

	v := transmission.DefaultMethodsValidator(downloadPrefix)

	eh := http_error.NewHttpErrHandler(debugMode)

	p := proxy(gw, eh)
	http.Handle(webPath, p)
	http.Handle(rpcPath, rpcProxy(p, v, eh))
	http.Handle("/", homePage(p))

	log.Fatalln(http.ListenAndServe(":8080", nil))
}
