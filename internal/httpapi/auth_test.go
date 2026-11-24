package httpapi

import (
	"net/http"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	for _, test := range []struct {
		raw, want string
		valid     bool
	}{
		{" User@Example.test ", "user@example.test", true},
		{"", "", false},
		{"not-an-email", "", false},
		{"Name <user@example.test>", "", false},
	} {
		t.Run(test.raw, func(t *testing.T) {
			email, valid := normalizeEmail(test.raw)
			if email != test.want || valid != test.valid {
				t.Fatalf("normalizeEmail = %q, %t", email, valid)
			}
		})
	}
}

func TestSessionTokens(t *testing.T) {
	first, firstHash, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	second, secondHash, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 64 || len(firstHash) != 64 || hashToken(first) != firstHash ||
		first == firstHash || first == second || firstHash == secondHash {
		t.Fatal("session tokens must be unique and stored as SHA-256 hashes")
	}
}

func TestDatabaseEndpointsWithoutDatabase(t *testing.T) {
	handler := NewRouter("", testDependencies())
	for _, test := range []struct{ method, path string }{
		{http.MethodPost, "/api/auth/register"},
		{http.MethodPost, "/api/auth/login"},
		{http.MethodPost, "/api/auth/logout"},
		{http.MethodGet, "/api/me"},
		{http.MethodGet, "/api/history"},
		{http.MethodGet, "/api/2gis/search?q=cafe&location=37.62,55.75"},
		{http.MethodGet, "/api/weather?location=37.62,55.75"},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := requestJSON(t, handler, test.method, test.path, "{}")
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("database-disabled status = %d, want 503", response.Code)
			}
		})
	}
}
