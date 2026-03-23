// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package tests

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/fred"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newMockFREDClient(t *testing.T, handlers map[string]http.HandlerFunc) *fred.Client {
	t.Helper()

	mux := http.NewServeMux()
	for path, h := range handlers {
		mux.HandleFunc(path, h)
	}

	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			return rec.Result(), nil
		}),
		Timeout: 5 * time.Second,
	}

	client := fred.NewClient("test_key", "https://mock.fred.local/", 5*time.Second, 1000, false)
	client.SetHTTPClient(httpClient)
	return client
}

func requireReachableHost(t *testing.T, hostport string) {
	t.Helper()

	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		t.Fatalf("invalid hostport %q: %v", hostport, err)
	}

	if _, err := net.LookupHost(host); err != nil {
		t.Skipf("⏭️  Skipping: DNS unavailable for %s (%v)", host, err)
	}

	conn, err := net.DialTimeout("tcp", hostport, 3*time.Second)
	if err != nil {
		t.Skipf("⏭️  Skipping: cannot reach %s (%v)", hostport, err)
	}
	if err := conn.Close(); err != nil {
		t.Logf("closing reachability probe connection: %v", err)
	}
}

func mustFormatHostPort(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}
