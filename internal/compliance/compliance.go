// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package compliance

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/derickschaefer/reserve/internal/config"
	"github.com/derickschaefer/reserve/internal/model"
	"github.com/derickschaefer/reserve/internal/store"
)

const (
	StatusUnknown                        = "unknown"
	StatusAmbiguousConflict              = "ambiguous_conflict"
	StatusCopyrightedPreapprovalRequired = "copyrighted_preapproval_required"
	StatusCopyrightedCitationRequired    = "copyrighted_citation_required"
	StatusPublicDomainCitationRequested  = "public_domain_citation_requested"

	internalBackfillComplete = "rights_backfill_completed"
)

// ResetBackfillState clears the one-time rights backfill marker so the next
// command run can rebuild the local rights index.
func ResetBackfillState(s *store.Store) error {
	if s == nil {
		return fmt.Errorf("store is required")
	}
	return s.DeleteInternalKey(internalBackfillComplete)
}

type SeriesClient interface {
	GetSeries(context.Context, string) (*model.SeriesMeta, error)
	GetSeriesTags(context.Context, string) ([]model.Tag, error)
}

type Decision struct {
	Allowed       bool
	BlockedReason string
	CitationText  string
}

func EnsureSeriesMeta(ctx context.Context, cfg *config.Config, client SeriesClient, s *store.Store, seriesID, action string) (model.SeriesMeta, bool, error) {
	seriesID = strings.ToUpper(strings.TrimSpace(seriesID))
	if seriesID == "" {
		return model.SeriesMeta{}, false, fmt.Errorf("series id required")
	}

	if s != nil && !cfg.Refresh {
		if meta, ok, err := s.GetSeriesMeta(seriesID); err == nil && ok {
			meta.PermissionOnFile = cfg.HasGrantedSeriesPermission(seriesID)
			if hasUsableRightsRecord(meta) && !NeedsRefresh(cfg, meta, action, time.Now()) {
				return meta, false, nil
			}
		} else if err != nil {
			return model.SeriesMeta{}, false, fmt.Errorf("reading local rights index for %s: %w", seriesID, err)
		}
	}
	if client == nil || isNilSeriesClient(client) {
		return model.SeriesMeta{}, false, fmt.Errorf("series %s requires live rights refresh but no API client is available", seriesID)
	}
	status := startDelayedStatus(ctx, cfg.Quiet, fmt.Sprintf("Checking permissions for %s...", seriesID))
	defer status.stop("")

	meta, err := client.GetSeries(ctx, seriesID)
	if err != nil {
		return model.SeriesMeta{}, false, err
	}
	tags, err := client.GetSeriesTags(ctx, seriesID)
	if err != nil {
		return model.SeriesMeta{}, false, err
	}
	enriched := EnrichSeriesMeta(*meta, tags)
	enriched.PermissionOnFile = cfg.HasGrantedSeriesPermission(seriesID)
	if s != nil {
		if err := s.PutSeriesMeta(enriched); err != nil {
			return model.SeriesMeta{}, true, fmt.Errorf("storing local rights index for %s: %w", seriesID, err)
		}
	}
	return enriched, true, nil
}

func isNilSeriesClient(client SeriesClient) bool {
	v := reflect.ValueOf(client)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

func NeedsRefresh(cfg *config.Config, meta model.SeriesMeta, action string, now time.Time) bool {
	if meta.LastRightsCheckAt.IsZero() {
		return true
	}
	maxAge := time.Duration(cfg.RightsRefreshDaysFor(action)) * 24 * time.Hour
	return now.Sub(meta.LastRightsCheckAt) > maxAge
}

func EnrichSeriesMeta(meta model.SeriesMeta, tags []model.Tag) model.SeriesMeta {
	status, rawTags, ambiguous := classifyRights(tags)
	meta.CopyrightStatus = status
	meta.RawRightsTags = rawTags
	meta.RightsAmbiguous = ambiguous
	meta.LastRightsCheckAt = time.Now().UTC()
	meta.SourceName = detectSourceName(tags)
	meta.CitationText = buildCitationText(meta)

	meta.PermissionRequired = status == StatusCopyrightedPreapprovalRequired
	meta.UsageAllowedCommercial = usageAllowed(status, "commercial", meta.PermissionOnFile)
	meta.UsageAllowedEducational = usageAllowed(status, "student", meta.PermissionOnFile)
	meta.UsageAllowedPersonal = usageAllowed(status, "personal", meta.PermissionOnFile)
	return meta
}

func Evaluate(cfg *config.Config, meta model.SeriesMeta, action string) Decision {
	citationRequired := RequiresCitation(meta)
	decision := Decision{
		Allowed:      true,
		CitationText: meta.CitationText,
	}

	switch meta.CopyrightStatus {
	case StatusUnknown:
		if cfg.BlockUnknownRights {
			decision.Allowed = false
			decision.BlockedReason = "rights metadata is unknown"
		}
	case StatusAmbiguousConflict:
		if cfg.BlockAmbiguousRights {
			decision.Allowed = false
			decision.BlockedReason = "rights metadata is ambiguous and requires review"
		}
	case StatusCopyrightedPreapprovalRequired:
		if meta.PermissionOnFile && cfg.AllowOverrideWithPermissionRecord {
			break
		}
		if cfg.PersonOrgType == "commercial" && cfg.BlockPreapprovalRequiredInCommercial {
			decision.Allowed = false
			decision.BlockedReason = "commercial use is blocked until permission is on file"
		} else {
			decision.Allowed = false
			decision.BlockedReason = "permission is required before this series can be used"
		}
	}

	if decision.Allowed && citationRequired && decision.CitationText == "" {
		switch action {
		case "display":
			if cfg.RequireCitationOnDisplay {
				decision.Allowed = false
				decision.BlockedReason = "citation metadata is missing"
			}
		case "export", "publish":
			if cfg.RequireCitationOnExport {
				decision.Allowed = false
				decision.BlockedReason = "citation metadata is missing"
			}
		}
	}

	if cfg.LogComplianceDecisions && cfg.Debug {
		slog.Info("compliance decision",
			"series_id", meta.ID,
			"rights_status", meta.CopyrightStatus,
			"usage_mode", cfg.PersonOrgType,
			"action", action,
			"allowed", decision.Allowed,
			"citation_attached", decision.CitationText != "",
			"permission_record_used", meta.PermissionOnFile,
		)
	}
	return decision
}

func RequiresCitation(meta model.SeriesMeta) bool {
	return meta.CopyrightStatus == StatusCopyrightedCitationRequired || meta.CopyrightStatus == StatusPublicDomainCitationRequested
}

func MaybeBackfillStore(ctx context.Context, cfg *config.Config, client SeriesClient, s *store.Store) error {
	if s == nil || client == nil {
		return nil
	}
	done, found, err := s.GetInternalBool(internalBackfillComplete)
	if err != nil {
		return fmt.Errorf("reading backfill state: %w", err)
	}
	if found && done {
		return nil
	}
	metas, err := s.ListSeriesMeta()
	if err != nil {
		return fmt.Errorf("listing stored series metadata: %w", err)
	}
	if len(metas) == 0 {
		if err := s.PutInternalBool(internalBackfillComplete, true); err != nil {
			return fmt.Errorf("recording backfill completion: %w", err)
		}
		return nil
	}

	status := startDelayedStatus(ctx, cfg.Quiet, fmt.Sprintf("Refreshing local rights index for %d cached series...", len(metas)))
	defer status.stop("Rights index refresh complete.")

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 8
	}
	sem := make(chan struct{}, concurrency)
	errCh := make(chan error, len(metas))
	var wg sync.WaitGroup
	for _, meta := range metas {
		meta := meta
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
			defer func() { <-sem }()
			if _, _, err := EnsureSeriesMeta(ctx, cfg, client, s, meta.ID, "display"); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	if err := s.PutInternalBool(internalBackfillComplete, true); err != nil {
		return fmt.Errorf("recording backfill completion: %w", err)
	}
	return nil
}

func FormatBlockedMessage(cfg *config.Config, meta model.SeriesMeta, decision Decision) string {
	reason := humanBlockedReason(meta, decision)
	return fmt.Sprintf(
		"Series %s is blocked for %s use.\n\nReason: %s\nHelp: https://reservecli.dev/documentation/permissions/",
		meta.ID,
		cfg.PersonOrgType,
		reason,
	)
}

func hasUsableRightsRecord(meta model.SeriesMeta) bool {
	return meta.CopyrightStatus != "" && !meta.LastRightsCheckAt.IsZero()
}

func classifyRights(tags []model.Tag) (string, []string, bool) {
	seen := map[string]struct{}{}
	var rights []string
	for _, tag := range tags {
		if tag.GroupID != "cc" {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(tag.Name))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		rights = append(rights, name)
	}
	slices.Sort(rights)
	if len(rights) == 0 {
		return StatusUnknown, nil, false
	}
	if len(rights) > 1 {
		return StatusAmbiguousConflict, rights, true
	}
	switch rights[0] {
	case "copyrighted: pre-approval required":
		return StatusCopyrightedPreapprovalRequired, rights, false
	case "copyrighted: citation required":
		return StatusCopyrightedCitationRequired, rights, false
	case "public domain: citation requested":
		return StatusPublicDomainCitationRequested, rights, false
	default:
		return StatusUnknown, rights, false
	}
}

func detectSourceName(tags []model.Tag) string {
	for _, tag := range tags {
		if tag.GroupID != "src" {
			continue
		}
		if strings.TrimSpace(tag.Notes) != "" {
			return strings.TrimSpace(tag.Notes)
		}
		if strings.TrimSpace(tag.Name) != "" {
			return strings.TrimSpace(tag.Name)
		}
	}
	return ""
}

func buildCitationText(meta model.SeriesMeta) string {
	if !RequiresCitation(meta) {
		return ""
	}
	source := strings.TrimSpace(meta.SourceName)
	if source == "" {
		source = "Unknown source"
	}
	return fmt.Sprintf("Source: %s via FRED", source)
}

func usageAllowed(status, personOrgType string, permissionOnFile bool) bool {
	switch status {
	case StatusPublicDomainCitationRequested, StatusCopyrightedCitationRequired:
		return true
	case StatusCopyrightedPreapprovalRequired:
		return permissionOnFile
	case StatusUnknown, StatusAmbiguousConflict:
		return false
	default:
		return false
	}
}

func humanBlockedReason(meta model.SeriesMeta, decision Decision) string {
	switch meta.CopyrightStatus {
	case StatusCopyrightedPreapprovalRequired:
		return "this series requires prior permission before use."
	case StatusAmbiguousConflict:
		return "this series has ambiguous rights metadata and requires review."
	case StatusUnknown:
		return "rights metadata for this series is unknown."
	default:
		if decision.BlockedReason == "" {
			return "this series cannot be used under the current permissions policy."
		}
		reason := strings.TrimSpace(decision.BlockedReason)
		if !strings.HasSuffix(reason, ".") {
			reason += "."
		}
		if strings.HasPrefix(strings.ToLower(reason), "this series") {
			return reason
		}
		return strings.ToLower(reason[:1]) + reason[1:]
	}
}

type delayedStatus struct {
	quiet   bool
	message string
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	printed bool
}

func startDelayedStatus(ctx context.Context, quiet bool, message string) *delayedStatus {
	s := &delayedStatus{
		quiet:   quiet,
		message: message,
		done:    make(chan struct{}),
	}
	if quiet {
		return s
	}
	go func() {
		timer := time.NewTimer(500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-timer.C:
			s.mu.Lock()
			s.printed = true
			s.mu.Unlock()
			fmt.Fprintln(os.Stderr, s.message)
		}
	}()
	return s
}

func (s *delayedStatus) stop(completion string) {
	s.once.Do(func() {
		close(s.done)
	})
	s.mu.Lock()
	printed := s.printed
	s.mu.Unlock()
	if printed && completion != "" && !s.quiet {
		fmt.Fprintln(os.Stderr, completion)
	}
}
