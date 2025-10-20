package httpmock

import (
	"net/http"
	"net/http/httptest"
)

// BaseURL is the canonical URL used by HTTP mocks. Tests should configure their configuration to point to this URL.
const BaseURL = "https://mock.newo.local"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// New constructs an HTTP client and transport that route requests to the supplied handler without opening network sockets.
// The returned client and transport can be injected into production code during tests to avoid relying on the network.
func New(handler http.Handler) (*http.Client, http.RoundTripper) {
	if handler == nil {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
	}

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		resp := rec.Result()
		resp.Request = req
		return resp, nil
	})

	return &http.Client{Transport: transport}, transport
}
