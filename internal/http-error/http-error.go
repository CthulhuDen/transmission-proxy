package http_error

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
)

type HttpErrHandler struct {
	debugMode bool
}

func NewHttpErrHandler(debugMode bool) *HttpErrHandler {
	return &HttpErrHandler{
		debugMode: debugMode,
	}
}

func (h *HttpErrHandler) Handle(w http.ResponseWriter, tag int, status int, msg string, logDetails ...any) {
	args := make([]any, 0, len(logDetails)+2)
	args = append(args, msg)
	args = append(args, logDetails...)

	var errId string

	if !h.debugMode {
		errId = uuid.NewString()
		args = append(args, "(err id: "+errId+" :)")
	}

	log.Println(args...)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	data := map[string]any{}

	if tag != 0 {
		data["tag"] = tag
	}

	if h.debugMode {
		s := fmt.Sprintln(args...) // fucking Sprint does not insert spaces between strings
		data["result"] = s[:len(s)-1]
	} else {
		data["result"] = "Unknown error occurred: " + errId
	}

	bs, err := json.Marshal(data)
	if err != nil {
		// WTF??
		log.Println("white marshalling error body for response:", err)
		bs = []byte("unknown error")
	}

	if _, err := fmt.Fprintln(w, string(bs)); err != nil {
		log.Println("while writing error body:", err)
	}
}
