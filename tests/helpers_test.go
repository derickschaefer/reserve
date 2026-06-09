// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package tests

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/model"
)

var (
	fredRequestMu      = make(chan struct{}, 1)
	lastFREDRequestAt  time.Time
	fredRequestSpacing = 1 * time.Second
)

func paceFREDRequests(t *testing.T) {
	t.Helper()

	fredRequestMu <- struct{}{}
	defer func() { <-fredRequestMu }()

	if !lastFREDRequestAt.IsZero() {
		wait := time.Until(lastFREDRequestAt.Add(fredRequestSpacing))
		if wait > 0 {
			time.Sleep(wait)
		}
	}
	lastFREDRequestAt = time.Now()
}

func pacedGetSeries(t *testing.T, client *fred.Client, seriesID string) (*model.SeriesMeta, error) {
	t.Helper()
	paceFREDRequests(t)
	return client.GetSeries(context.Background(), seriesID)
}

func pacedGetSeriesTags(t *testing.T, client *fred.Client, seriesID string) ([]model.Tag, error) {
	t.Helper()
	paceFREDRequests(t)
	return client.GetSeriesTags(context.Background(), seriesID)
}

func pacedGetObservations(t *testing.T, client *fred.Client, seriesID string, opts fred.ObsOptions) (*model.SeriesData, error) {
	t.Helper()
	paceFREDRequests(t)
	return client.GetObservations(context.Background(), seriesID, opts)
}

func pacedGetLatestObservation(t *testing.T, client *fred.Client, seriesID string) (*model.Observation, error) {
	t.Helper()
	paceFREDRequests(t)
	return client.GetLatestObservation(context.Background(), seriesID)
}

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
