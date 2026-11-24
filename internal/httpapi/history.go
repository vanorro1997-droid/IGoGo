package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vanorro1997-droid/IGoGo/internal/storage"
)

// statusWriter запоминает статус ответа, чтобы записать его в историю запросов.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wrote {
		return
	}
	w.status = code
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func nullString(s string) sql.NullString {
	s = strings.TrimSpace(s)
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func clientIP(remote string) sql.NullString {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	return nullString(host)
}

func providerForRequest(r *http.Request) sql.NullString {
	switch r.URL.Path {
	case "/api/2gis/search":
		return nullString("2gis")
	case "/api/weather":
		if r.URL.Query().Has("location") {
			return nullString("weatherapi")
		}
	}
	return sql.NullString{}
}

// logRequests пишет в api_request_log каждый запрос авторизованного пользователя.
func logRequests(store *storage.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)

		user, ok := userFromContext(r.Context())
		if !ok {
			return
		}

		query := r.URL.Query()
		for _, key := range []string{"token", "password", "key", "api_key"} {
			query.Del(key)
		}
		params, _ := json.Marshal(query)

		entry := storage.RequestLog{
			UserID:      user.ID,
			Method:      r.Method,
			Path:        r.URL.Path,
			QueryParams: params,
			Provider:    providerForRequest(r),
			StatusCode:  recorder.status,
			DurationMS:  int(time.Since(started).Milliseconds()),
			IP:          clientIP(r.RemoteAddr),
			UserAgent:   nullString(r.UserAgent()),
		}
		if recorder.status >= 400 {
			entry.ErrorMessage = nullString(http.StatusText(recorder.status))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := store.LogRequest(ctx, entry); err != nil {
			log.Printf("history: failed to log request: %v", err)
		}
	})
}

// GET /api/history?limit=50 — история запросов текущего пользователя.
func (s *apiServer) historyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	user, ok := userFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	items, err := s.store.ListRequests(ctx, user.ID, limit)
	if err != nil {
		log.Printf("history: failed to list requests: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load history")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
