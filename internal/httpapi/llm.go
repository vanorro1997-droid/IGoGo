package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

type LLMClient interface {
	AskCtx(context.Context, string) (string, error)
	Model() string
}

func makeLLMHandler(client LLMClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "llm is disabled"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var request struct {
			Prompt string `json:"prompt"`
		}
		if err := decoder.Decode(&request); err != nil {
			writeInvalidLLMBody(w, err)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeInvalidLLMBody(w, err)
			return
		}
		prompt := strings.TrimSpace(request.Prompt)
		if prompt == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt must not be empty"})
			return
		}

		text, err := client.AskCtx(r.Context(), prompt)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "llm provider failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"text": text, "model": client.Model()})
	}
}

func writeInvalidLLMBody(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body exceeds 16 KiB"})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be a single JSON object with a prompt field"})
}
