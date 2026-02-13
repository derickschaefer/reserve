// Package app wires together configuration, the API client, and the local
// store into a single Deps struct that commands receive at runtime.
package app

import (
	"fmt"

	"github.com/derickschaefer/reserve/internal/config"
	"github.com/derickschaefer/reserve/internal/fred"
	"github.com/derickschaefer/reserve/internal/store"
)

// Deps holds all runtime dependencies injected into command Run functions.
type Deps struct {
	Config *config.Config
	Client *fred.Client
	Store  *store.Store // nil if DB could not be opened
}

// New builds a Deps from resolved config.
// Store open failures are non-fatal: Deps.Store will be nil and live API
// commands still work. Commands that require persistence call RequireStore().
func New(cfg *config.Config) *Deps {
	client := fred.NewClient(
		cfg.APIKey,
		cfg.BaseURL,
		cfg.Timeout,
		cfg.Rate,
		cfg.Debug,
	)
	d := &Deps{
		Config: cfg,
		Client: client,
	}
	if cfg.DBPath != "" {
		if s, err := store.Open(cfg.DBPath); err == nil {
			d.Store = s
		}
		// Silently continue — live API commands work without the store.
	}
	return d
}

// RequireStore returns an error if the store is not available.
// Call this at the top of any command that needs persistence.
func (d *Deps) RequireStore() error {
	if d.Store == nil {
		return fmt.Errorf(
			"local database unavailable\n\n" +
				"Check that db_path is set in config.json or RESERVE_DB_PATH env var.\n" +
				"Default path: ~/.reserve/reserve.db",
		)
	}
	return nil
}

// Close releases the store if open. Safe to call even if Store is nil.
func (d *Deps) Close() {
	if d.Store != nil {
		d.Store.Close()
	}
}
