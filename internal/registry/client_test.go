// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBrowseAndCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/browse" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"schema_version":1,"repository":{"id":"official","name":"reserve-workflows"},"workflows":[{"id":"cpi-history","collection":"inflation","title":"CPI History","latest_version":"1.0.0"}]}`))
			return
		}
		if r.URL.Path == "/v1/collections/inflation" {
			_, _ = w.Write([]byte(`{"id":"inflation","workflow_count":1,"workflows":[{"id":"cpi-history","title":"CPI History","latest_version":"1.0.0"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL+"/v1", time.Second)
	catalog, err := client.Browse(context.Background())
	if err != nil || len(catalog.Workflows) != 1 || catalog.Workflows[0].ID != "cpi-history" {
		t.Fatalf("Browse() = %#v, %v", catalog, err)
	}
	collection, err := client.Collection(context.Background(), "inflation")
	if err != nil || collection.WorkflowCount != 1 {
		t.Fatalf("Collection() = %#v, %v", collection, err)
	}
}

func TestWorkflowSendsBearerTokenAndReadsVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer reserve-test-token" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("X-Reserve-Workflow-Version", "1.0.0")
		_, _ = w.Write([]byte("workflow:\n  title: Test\n"))
	}))
	defer server.Close()

	client := NewClient(server.URL, time.Second)
	client.Token = "reserve-test-token"
	artifact, err := client.Workflow(context.Background(), "inflation", "cpi-history", "")
	if err != nil || artifact.Version != "1.0.0" || !strings.Contains(string(artifact.Content), "title: Test") {
		t.Fatalf("Workflow() = %#v, %v", artifact, err)
	}
}
