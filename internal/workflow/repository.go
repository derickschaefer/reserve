// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package workflow

// RepositoryType identifies the trust and ownership boundary for a workflow
// repository. Only the built-in official repository is represented here for
// v1.2.1; local and private repositories remain future-facing concepts.
type RepositoryType string

const (
	RepositoryTypeOfficial RepositoryType = "official"
	RepositoryTypeLocal    RepositoryType = "local"
	RepositoryTypePrivate  RepositoryType = "private"
)

const (
	OfficialRepositoryID       = "official"
	OfficialRepositoryName     = "reserve-workflows"
	OfficialRepositoryEndpoint = "https://api.reservecli.dev/v1"
)

// Repository is the internal identity used when a workflow source is known.
// Endpoint describes the RESERVE distribution boundary; callers should not
// need to know how the official repository is stored or published.
type Repository struct {
	ID       string
	Name     string
	Type     RepositoryType
	Endpoint string
}

// OfficialRepository returns the built-in public repository descriptor.
func OfficialRepository() Repository {
	return Repository{
		ID:       OfficialRepositoryID,
		Name:     OfficialRepositoryName,
		Type:     RepositoryTypeOfficial,
		Endpoint: OfficialRepositoryEndpoint,
	}
}
