package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewDefaultsAndTransportIsolation(t *testing.T) {
	defaultTransport := http.DefaultTransport
	client, err := New(Config{APIKey: "test-token", AccountID: "test-account"})
	if err != nil {
		t.Fatal(err)
	}
	if client.Model() != "@cf/google/gemma-4-26b-a4b-it" || client.cfg.Timeout != 60*time.Second {
		t.Fatal("existing model and timeout defaults must be preserved")
	}
	if http.DefaultTransport != defaultTransport || client.httpClient == nil || client.httpClient.Transport == defaultTransport {
		t.Fatal("LLM must use an isolated HTTP transport")
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	for _, test := range []struct {
		name string
		cfg  Config
	}{
		{"missing token", Config{AccountID: "test-account"}},
		{"missing account", Config{APIKey: "test-token"}},
		{"negative timeout", Config{APIKey: "test-token", AccountID: "test-account", Timeout: -time.Second}},
		{"missing proxy scheme", Config{APIKey: "test-token", AccountID: "test-account", Proxy: "localhost:10808"}},
		{"invalid proxy", Config{APIKey: "test-token", AccountID: "test-account", Proxy: "http://%"}},
		{"unsupported proxy", Config{APIKey: "test-token", AccountID: "test-account", Proxy: "ftp://localhost:10808"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.cfg); err == nil {
				t.Fatal("expected invalid configuration to be rejected")
			}
		})
	}
}

func TestNewAcceptsSupportedProxies(t *testing.T) {
	for _, scheme := range []string{"http", "https", "socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			client, err := New(Config{APIKey: "test-token", AccountID: "test-account", Proxy: scheme + "://localhost:10808"})
			if err != nil {
				t.Fatal(err)
			}
			proxy, err := client.httpClient.Transport.(*http.Transport).Proxy(&http.Request{})
			if err != nil || proxy == nil || proxy.Scheme != scheme {
				t.Fatal("proxy was not configured on the isolated transport")
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testClient(t *testing.T) *Client {
	t.Helper()
	t.Setenv("CLOUDFLARE_BASE_URL", "")
	client, err := New(Config{APIKey: "test-token", AccountID: "test-account", SystemPrompt: "test system prompt"})
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer test-token" ||
			request.URL.Path != "/client/v4/accounts/test-account/ai/v1/chat/completions" {
			t.Error("Cloudflare credentials and account were not forwarded through the custom client")
		}
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Messages) != 2 || body.Messages[0].Role != "system" || body.Messages[0].Content != "test system prompt" ||
			body.Messages[1].Role != "user" || body.Messages[1].Content != "test prompt" {
			t.Error("system and user prompts were not preserved")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"test","choices":[{"index":0,"message":{"role":"assistant","content":"test answer"},"finish_reason":"stop"}]}`)),
		}, nil
	})
	return client
}

func TestAskCtxUsesCustomHTTPClient(t *testing.T) {
	client := testClient(t)
	text, err := client.AskCtx(context.Background(), "test prompt")
	if err != nil || text != "test answer" {
		t.Fatalf("text = %q, error = %v", text, err)
	}
}

func TestAskAsyncClosesResultChannel(t *testing.T) {
	client := testClient(t)
	results := client.AskAsync("test prompt")
	result := <-results
	if result.Err != nil || result.Text != "test answer" {
		t.Fatalf("unexpected asynchronous result: %v", result.Err)
	}
	if _, open := <-results; open {
		t.Fatal("result channel must close after one response")
	}
}

func TestAskCtxCancellation(t *testing.T) {
	client := testClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client.httpClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, request.Context().Err()
	})
	if _, err := client.AskCtx(ctx, "test prompt"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
