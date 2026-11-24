package storage

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestDatabaseDSN(t *testing.T) {
	t.Setenv("DB_HOST", "::1")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "test@user")
	t.Setenv("DB_PASSWORD", "test:/?#password")
	t.Setenv("DB_NAME", "test database")
	t.Setenv("DB_SSLMODE", "require")
	dsn, err := url.Parse(dbDSN())
	if err != nil {
		t.Fatal(err)
	}
	password, _ := dsn.User.Password()
	if dsn.Hostname() != "::1" || dsn.Port() != "5433" || dsn.User.Username() != "test@user" ||
		password != "test:/?#password" || dsn.Path != "/test database" || dsn.Query().Get("sslmode") != "require" {
		t.Fatal("database settings must be escaped without changing their values")
	}
}

func TestExternalIDsPreservePrecision(t *testing.T) {
	for _, test := range []struct{ raw, want string }{
		{`"9007199254740993"`, "9007199254740993"},
		{`9007199254740993`, "9007199254740993"},
		{`"external-place"`, "external-place"},
		{"", ""},
		{"invalid", ""},
	} {
		if got := parseExternalID(json.RawMessage(test.raw)); got != test.want {
			t.Errorf("parseExternalID(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}

func TestUniqueViolation(t *testing.T) {
	if !IsUniqueViolation(fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23505"})) ||
		IsUniqueViolation(&pgconn.PgError{Code: "23503"}) || IsUniqueViolation(nil) {
		t.Fatal("only unique constraint violations should map to duplicate email errors")
	}
}
