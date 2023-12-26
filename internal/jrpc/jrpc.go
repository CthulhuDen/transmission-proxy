package jrpc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Request struct {
	Method    string                 `json:"method"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
	Tag       int                    `json:"tag,omitempty"`
}

//type Response struct {
//	Result    string
//	Arguments map[string]interface{}
//	Tag       int
//}

func FromRequest(r *http.Request) (*Request, error) {
	defer func() { _ = r.Body.Close() }()

	bs, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("reading request body: %w", err)
	}

	req := Request{}
	if err = json.Unmarshal(bs, &req); err != nil {
		return nil, fmt.Errorf("parsing request body: %w", err)
	}

	return &req, nil
}
