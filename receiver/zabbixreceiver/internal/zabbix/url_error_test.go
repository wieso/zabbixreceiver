package zabbix

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestCanceledRequestRedactsURLCredentials(t *testing.T) {
	client := newTestClient(t, "http://review-user:review-password@example.invalid/api?access_token=review-query-secret#review-fragment")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Hosts(ctx)
	for _, secret := range []string{"review-user", "review-password", "review-query-secret", "review-fragment"} {
		requireErrorChainNotContains(t, err, secret)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Fatalf("lost URL error type: %v", err)
	}
	requireEqual(t, "http://example.invalid/api", urlErr.URL)
}

func TestRedirectErrorRedactsDestinationURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.invalid/next?secret=redirect-secret", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{URL: server.URL + "?secret=original-secret", Token: testToken}, &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return context.DeadlineExceeded },
	})
	requireNoError(t, err)
	_, err = client.Hosts(context.Background())
	requireErrorChainNotContains(t, err, "redirect-secret")
	requireErrorChainNotContains(t, err, "original-secret")
	var urlErr *url.Error
	if !errors.As(err, &urlErr) || !urlErr.Timeout() || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lost timeout semantics: %v", err)
	}
}

func TestWrappedURLErrorRedactsParentAndCause(t *testing.T) {
	original := &url.Error{Op: "Post", URL: "http://user:password@example.invalid/api?key=query-secret#fragment-secret", Err: context.Canceled}
	err := redactError(fmt.Errorf("request failed: %w", original), testToken)
	for _, secret := range []string{"user", "password", "query-secret", "fragment-secret"} {
		requireErrorChainNotContains(t, err, secret)
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) || !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cause: %v", err)
	}
	requireEqual(t, original.URL, "http://user:password@example.invalid/api?key=query-secret#fragment-secret")
}

func TestMalformedRedirectDoesNotLeakLocation(t *testing.T) {
	for _, location := range []string{"http://example.invalid/%zz?key=redirect-secret", "/%zz?key=redirect-secret"} {
		t.Run(location, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", location)
				w.WriteHeader(http.StatusFound)
			}))
			defer server.Close()
			_, err := newTestClient(t, server.URL).Hosts(context.Background())
			if err == nil {
				t.Fatal("expected malformed redirect error")
			}
			requireErrorChainNotContains(t, err, "redirect-secret")
		})
	}
}
