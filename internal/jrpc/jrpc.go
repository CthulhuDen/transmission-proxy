package jrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Request struct {
	Method    string                 `json:"method"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
	Tag       int                    `json:"tag,omitempty"`
	Context   context.Context        `json:"-"`
}

func FromRequest(r *http.Request) (*Request, error) {
	defer func() { _ = r.Body.Close() }()

	bs, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	req := Request{}
	if err = json.Unmarshal(bs, &req); err != nil {
		return nil, fmt.Errorf("parse body: %w", err)
	}

	req.Context = r.Context()
	return &req, nil
}
