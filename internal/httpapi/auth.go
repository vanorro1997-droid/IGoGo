package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vanorro1997-droid/IGoGo/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const (
	userContextKey        contextKey = "user"
	sessionHashContextKey contextKey = "session_hash"
)

var dummyPasswordHash = func() []byte {
	hash, _ := bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)
	return hash
}()

type authService struct {
	store      *storage.Store
	sessionTTL time.Duration
}

func newAuthService(store *storage.Store) *authService {
	hours := 168 // 7 дней по умолчанию
	if v := strings.TrimSpace(os.Getenv("SESSION_TTL_HOURS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			hours = n
		}
	}
	return &authService{store: store, sessionTTL: time.Duration(hours) * time.Hour}
}

func decodeJSONBody(r *http.Request, dst any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func normalizeEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > 254 {
		return "", false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || !strings.Contains(email, "@") {
		return "", false
	}
	return email, true
}

func newSessionToken() (token, tokenHash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(buf)
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:]), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func userFromContext(ctx context.Context) (storage.User, bool) {
	user, ok := ctx.Value(userContextKey).(storage.User)
	return user, ok
}

func sessionHashFromContext(ctx context.Context) (string, bool) {
	hash, ok := ctx.Value(sessionHashContextKey).(string)
	return hash, ok
}

func (a *authService) authResponse(w http.ResponseWriter, status int, user storage.User, token string, expiresAt time.Time) {
	writeJSON(w, status, map[string]any{
		"user":       user,
		"token":      token,
		"token_type": "Bearer",
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
	})
}

// POST /api/auth/register — создаёт пользователя и сразу выдаёт сессию.
func (a *authService) registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	email, ok := normalizeEmail(req.Email)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid email")
		return
	}
	if len(req.Password) < 8 {
		writeJSONError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	user, err := a.store.CreateUser(ctx, email, string(passwordHash), strings.TrimSpace(req.DisplayName))
	if err != nil {
		if storage.IsUniqueViolation(err) {
			writeJSONError(w, http.StatusConflict, "email already registered")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	token, tokenHash, err := newSessionToken()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	expiresAt := time.Now().UTC().Add(a.sessionTTL)
	if err := a.store.CreateSession(ctx, user.ID, tokenHash, expiresAt); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	_ = a.store.UpdateLastLogin(ctx, user.ID)

	a.authResponse(w, http.StatusCreated, user, token, expiresAt)
}

// POST /api/auth/login — проверяет пароль и создаёт новую сессию.
func (a *authService) loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	email, ok := normalizeEmail(req.Email)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid email")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	user, passwordHash, err := a.store.UserByEmail(ctx, email)
	hashToCheck := dummyPasswordHash
	if err == nil {
		hashToCheck = []byte(passwordHash)
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusInternalServerError, "failed to authenticate")
		return
	}

	if compareErr := bcrypt.CompareHashAndPassword(hashToCheck, []byte(req.Password)); compareErr != nil || err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token, tokenHash, err := newSessionToken()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	expiresAt := time.Now().UTC().Add(a.sessionTTL)
	if err := a.store.CreateSession(ctx, user.ID, tokenHash, expiresAt); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	_ = a.store.UpdateLastLogin(ctx, user.ID)

	a.authResponse(w, http.StatusOK, user, token, expiresAt)
}

// POST /api/auth/logout — отзывает текущую сессию.
func (a *authService) logoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	hash, ok := sessionHashFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := a.store.RevokeSession(ctx, hash); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to revoke session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /api/me — данные авторизованного пользователя из app_user.
func (a *authService) meHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	user, ok := userFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

// requireAuth пропускает только запросы с валидным Bearer-токеном.
func (a *authService) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeJSONError(w, http.StatusUnauthorized, "authorization required")
			return
		}

		tokenHash := hashToken(parts[1])
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		user, err := a.store.SessionUser(ctx, tokenHash)
		cancel()
		if err != nil {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeJSONError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		ctx = context.WithValue(r.Context(), userContextKey, user)
		ctx = context.WithValue(ctx, sessionHashContextKey, tokenHash)
		*r = *r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}
