package httpx

import (
	"net/http"
	"testing"
	"time"
)

func TestNewClientDefaults(t *testing.T) {
	client := NewClient()
	if client.Timeout != 30*time.Second {
		t.Fatalf("Timeout = %s, want 30s", client.Timeout)
	}
}

func TestUserAgentTransport(t *testing.T) {
	var got string
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		got = req.Header.Get("User-Agent")
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	})

	client := NewClientWithTransport(base)
	req, err := http.NewRequest(http.MethodGet, "https://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if got != UserAgent {
		t.Fatalf("User-Agent = %q, want %q", got, UserAgent)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
