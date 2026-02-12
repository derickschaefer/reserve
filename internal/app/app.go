// Package app wires together configuration, the API client, and other
// dependencies into a single Deps struct that commands receive at runtime.
package app

import (
	"github.com/derickschaefer/reserve/internal/config"
	"github.com/derickschaefer/reserve/internal/fred"
)

// Deps holds all runtime dependencies injected into command Run functions.
// Phase 1 holds Config and Client; Phase 3 will add Store and Cache.
type Deps struct {
	Config *config.Config
	Client *fred.Client
}

// New builds a Deps from resolved config.
func New(cfg *config.Config) *Deps {
	client := fred.NewClient(
		cfg.APIKey,
		cfg.BaseURL,
		cfg.Timeout,
		cfg.Rate,
		cfg.Debug,
	)
	return &Deps{
		Config: cfg,
		Client: client,
	}
}
