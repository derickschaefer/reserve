// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package workflow

import "testing"

func TestOfficialRepositoryDescriptor(t *testing.T) {
	repo := OfficialRepository()
	if repo.ID != OfficialRepositoryID || repo.Name != OfficialRepositoryName {
		t.Fatalf("unexpected official repository identity: %#v", repo)
	}
	if repo.Type != RepositoryTypeOfficial {
		t.Fatalf("repository type = %q, want %q", repo.Type, RepositoryTypeOfficial)
	}
	if OfficialRepositoryEndpoint != "https://api.reservecli.dev/v1" {
		t.Fatalf("official repository constant = %q, want https://api.reservecli.dev/v1", OfficialRepositoryEndpoint)
	}
	if repo.Endpoint != OfficialRepositoryEndpoint {
		t.Fatalf("official repository endpoint = %q, want https://api.reservecli.dev/v1", repo.Endpoint)
	}
}
