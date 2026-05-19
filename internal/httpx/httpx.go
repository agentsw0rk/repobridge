package httpx

import (
	"net/http"
	"time"
)

const UserAgent = "repobridge-cli"

type userAgentTransport struct {
	base http.RoundTripper
}

func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	if cloned.Header.Get("User-Agent") == "" {
		cloned.Header.Set("User-Agent", UserAgent)
	}
	return t.base.RoundTrip(cloned)
}

func NewClient() *http.Client {
	return NewClientWithTransport(http.DefaultTransport)
}

func NewClientWithTransport(base http.RoundTripper) *http.Client {
	if base == nil {
		base = http.DefaultTransport
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: userAgentTransport{base: base},
	}
}
