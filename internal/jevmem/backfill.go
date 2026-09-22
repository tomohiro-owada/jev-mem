package jevmem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type BackfillOptions struct {
	RepoDir string
	Limit   int
	DryRun  bool
}

type BackfillProgress struct {
	Processed int           `json:"processed"`
	Total     int           `json:"total"`
	Updated   int           `json:"updated"`
	Skipped   int           `json:"skipped"`
	Failed    int           `json:"failed"`
	Path      string        `json:"path"`
	Elapsed   time.Duration `json:"-"`
	ETA       time.Duration `json:"-"`
	Error     string        `json:"error,omitempty"`
}

type BackfillResult struct {
	Candidates int      `json:"candidates"`
	Processed  int      `json:"processed"`
	Updated    int      `json:"updated"`
	Skipped    int      `json:"skipped"`
	Failed     int      `json:"failed"`
	Failures   []string `json:"failures"`
	DryRun     bool     `json:"dry_run"`
	CommitHash string   `json:"commit_hash,omitempty"`
	Pushed     bool     `json:"pushed"`
}

func BackfillLegacyFingerprints(ctx context.Context, extractor FingerprintExtractor, options BackfillOptions, progress func(BackfillProgress)) (BackfillResult, error) {
	if extractor == nil {
		return BackfillResult{}, fmt.Errorf("fingerprint extractor is required")
	}
	if strings.TrimSpace(options.RepoDir) == "" {
		return BackfillResult{}, fmt.Errorf("repo directory is required")
	}
	var paths []string
	err := filepath.WalkDir(options.RepoDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return BackfillResult{}, err
	}
	sort.Strings(paths)
	type candidate struct {
		path     string
		rel      string
		raw      string
		decision DecisionInput
	}
	candidates := make([]candidate, 0, len(paths))
	result := BackfillResult{DryRun: options.DryRun, Failures: []string{}}
	for _, path := range paths {
		rawBytes, readErr := os.ReadFile(path)
		if readErr != nil {
			return result, readErr
		}
		rel, _ := filepath.Rel(options.RepoDir, path)
		_, existing := splitFingerprint(string(rawBytes))
		if existing != nil {
			result.Skipped++
			continue
		}
		decision, ok := ParseLegacyDecision(rel, string(rawBytes))
		if !ok {
			continue
		}
		candidates = append(candidates, candidate{path: path, rel: rel, raw: string(rawBytes), decision: decision})
	}
	if options.Limit > 0 && len(candidates) > options.Limit {
		candidates = candidates[:options.Limit]
	}
	result.Candidates = len(candidates)
	started := time.Now()
	for index, item := range candidates {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		fingerprint, extractErr := extractor.Extract(ctx, item.decision)
		var itemErr error
		if extractErr != nil {
			itemErr = extractErr
		} else if !options.DryRun {
			var updated string
			updated, itemErr = AttachFingerprint(item.raw, fingerprint)
			if itemErr == nil {
				itemErr = os.WriteFile(item.path, []byte(updated), 0o644)
			}
		}
		result.Processed++
		if itemErr != nil {
			result.Failed++
			result.Failures = append(result.Failures, item.rel+": "+itemErr.Error())
		} else {
			result.Updated++
		}
		elapsed := time.Since(started)
		eta := time.Duration(0)
		if result.Processed > 0 {
			remaining := len(candidates) - result.Processed
			eta = time.Duration(int64(elapsed) * int64(remaining) / int64(result.Processed))
		}
		if progress != nil {
			p := BackfillProgress{Processed: index + 1, Total: len(candidates), Updated: result.Updated, Skipped: result.Skipped, Failed: result.Failed, Path: item.rel, Elapsed: elapsed, ETA: eta}
			if itemErr != nil {
				p.Error = itemErr.Error()
			}
			progress(p)
		}
	}
	return result, nil
}
