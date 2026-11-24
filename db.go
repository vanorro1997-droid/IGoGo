package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Store инкапсулирует доступ к PostgreSQL (.driver pgx через database/sql).
type Store struct {
	db *sql.DB
}

// User — авторизованный пользователь, возвращаемый в API и в контексте запроса.
type User struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// RequestHistoryItem — запись истории, отдаваемая клиенту.
type RequestHistoryItem struct {
	ID          int64           `json:"id"`
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	QueryParams json.RawMessage `json:"query_params"`
	Provider    *string         `json:"provider"`
	StatusCode  int             `json:"status_code"`
	DurationMS  int             `json:"duration_ms"`
	CreatedAt   time.Time       `json:"created_at"`
}

// RequestLog — запись истории обращения пользователя к API.
type RequestLog struct {
	UserID       int64
	Method       string
	Path         string
	QueryParams  []byte
	Provider     sql.NullString
	StatusCode   int
	DurationMS   int
	IP           sql.NullString
	UserAgent    sql.NullString
	ErrorMessage sql.NullString
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func dbDSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(env("DB_USER", "postgres"), env("DB_PASSWORD", "postgres")),
		Host:   net.JoinHostPort(env("DB_HOST", "localhost"), env("DB_PORT", "5432")),
		Path:   "/" + env("DB_NAME", "igogo"),
	}
	q := u.Query()
	q.Set("sslmode", env("DB_SSLMODE", "disable"))
	u.RawQuery = q.Encode()
	return u.String()
}

func openStore() (*Store, error) {
	db, err := sql.Open("pgx", dbDSN())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// CreateUser сохраняет нового пользователя в app_user.
func (s *Store) CreateUser(ctx context.Context, email, passwordHash, displayName string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO app_user (email, password_hash, display_name)
		VALUES ($1, $2, NULLIF($3, ''))
		RETURNING id, email, COALESCE(display_name, ''),
		          (SELECT code FROM user_role WHERE id = role_id)
	`, email, passwordHash, displayName).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role)
	return user, err
}

// UserByEmail возвращает пользователя и хэш пароля для проверки при логине.
func (s *Store) UserByEmail(ctx context.Context, email string) (User, string, error) {
	var user User
	var passwordHash string
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.email, COALESCE(u.display_name, ''),
		       (SELECT code FROM user_role WHERE id = u.role_id), u.password_hash
		FROM app_user u
		WHERE u.email = $1
	`, email).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &passwordHash)
	return user, passwordHash, err
}

// CreateSession сохраняет хэш токена в user_session.
func (s *Store) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_session (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, expiresAt)
	return err
}

// SessionUser проверяет активную сессию и возвращает её владельца.
func (s *Store) SessionUser(ctx context.Context, tokenHash string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.email, COALESCE(u.display_name, ''),
		       (SELECT code FROM user_role WHERE id = u.role_id)
		FROM user_session s
		JOIN app_user u ON u.id = s.user_id
		JOIN user_role r ON r.id = u.role_id
		WHERE s.token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > now()
		  AND u.status = 'active'
	`, tokenHash).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role)
	return user, err
}

func (s *Store) RevokeSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_session SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash)
	return err
}

func (s *Store) UpdateLastLogin(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE app_user SET last_login_at = now() WHERE id = $1`, userID)
	return err
}

// LogRequest пишет историю запросов в api_request_log.
func (s *Store) LogRequest(ctx context.Context, entry RequestLog) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO api_request_log
		    (user_id, method, path, query_params, provider, status_code,
		     duration_ms, ip, user_agent, error_message)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8, $9, $10)
	`,
		entry.UserID, entry.Method, entry.Path, entry.QueryParams,
		entry.Provider, entry.StatusCode, entry.DurationMS,
		entry.IP, entry.UserAgent, entry.ErrorMessage,
	)
	return err
}

// ListRequests возвращает последние запросы пользователя из api_request_log.
func (s *Store) ListRequests(ctx context.Context, userID int64, limit int) ([]RequestHistoryItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, method, path, query_params, provider, status_code, duration_ms, created_at
		FROM api_request_log
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]RequestHistoryItem, 0, limit)
	for rows.Next() {
		var item RequestHistoryItem
		var params []byte
		var provider sql.NullString
		if err := rows.Scan(&item.ID, &item.Method, &item.Path, &params,
			&provider, &item.StatusCode, &item.DurationMS, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.QueryParams = json.RawMessage(params)
		if provider.Valid {
			value := provider.String
			item.Provider = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
