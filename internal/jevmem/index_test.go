package jevmem

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestIndexSearch(t *testing.T) {
	ctx := context.Background()
	idx, err := OpenIndex(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	emb := HashEmbedder{}
	vec, err := emb.Embed(ctx, "query: alpha")
	if err != nil {
		t.Fatal(err)
	}
	mem := Memory{
		ProjectID: "proj",
		Path:      "projects/proj/a.md",
		Title:     "Alpha",
		Content:   "alpha content",
		Embedding: vec,
		Hash:      ContentHash("Alpha", "alpha content"),
		CreatedAt: time.Now(),
	}
	if err := idx.Upsert(ctx, mem, "test", "hash"); err != nil {
		t.Fatal(err)
	}
	results, err := idx.Search(ctx, vec, "proj", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Title != "Alpha" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestIndexSearchFingerprint(t *testing.T) {
	ctx := context.Background()
	idx, err := OpenIndex(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	emb := HashEmbedder{}
	vec, err := emb.Embed(ctx, "decision")
	if err != nil {
		t.Fatal(err)
	}
	query := Fingerprint{SchemaVersion: 1, Values: map[string]AttributeValue{
		"optionality_priority": fpValue(90, 1),
		"delay_cost":           fpValue(10, 1),
	}}
	near := query
	far := Fingerprint{SchemaVersion: 1, Values: map[string]AttributeValue{
		"optionality_priority": fpValue(10, 1),
		"delay_cost":           fpValue(90, 1),
	}}
	for _, mem := range []Memory{
		{ProjectID: "proj", Path: "projects/proj/near.md", Title: "Near", Content: "near", Embedding: vec, Fingerprint: &near, Hash: "near", CreatedAt: time.Now()},
		{ProjectID: "proj", Path: "projects/proj/far.md", Title: "Far", Content: "far", Embedding: vec, Fingerprint: &far, Hash: "far", CreatedAt: time.Now()},
	} {
		if err := idx.Upsert(ctx, mem, "test", "hash"); err != nil {
			t.Fatal(err)
		}
	}
	results, err := idx.SearchFingerprint(ctx, query, "proj", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Title != "Near" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if results[0].Similarity.Score <= results[1].Similarity.Score {
		t.Fatalf("scores are not ordered: %+v", results)
	}
}

func TestIndexSearchAnalogiesCanPreferCrossDomainStructure(t *testing.T) {
	ctx := context.Background()
	idx, err := OpenIndex(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	query := Fingerprint{SchemaVersion: 1, Values: map[string]AttributeValue{
		"state_uncertainty":             fpValue(80, 1),
		"future_option_foreclosure":     fpValue(90, 1),
		"optionality_priority":          fpValue(100, 1),
		"reversibility_priority":        fpValue(90, 1),
		"precommitment_learning_extent": fpValue(100, 1),
		"retained_option_extent":        fpValue(100, 1),
	}}
	structurallyNear := query
	semanticallyNear := Fingerprint{SchemaVersion: 1, Values: map[string]AttributeValue{
		"state_uncertainty":             fpValue(10, 1),
		"future_option_foreclosure":     fpValue(10, 1),
		"optionality_priority":          fpValue(10, 1),
		"reversibility_priority":        fpValue(10, 1),
		"precommitment_learning_extent": fpValue(0, 1),
		"retained_option_extent":        fpValue(0, 1),
	}}
	lowCoverage := Fingerprint{SchemaVersion: 1, Values: map[string]AttributeValue{
		"optionality_priority": fpValue(100, 1),
	}}

	memories := []Memory{
		{ProjectID: "proj", Path: "projects/proj/db.md", Title: "Database migration", Content: "database selection", Embedding: []float32{1, 0}, Fingerprint: &semanticallyNear, Hash: "db", CreatedAt: time.Now()},
		{ProjectID: "proj", Path: "projects/proj/hiring.md", Title: "Hiring decision", Content: "candidate decision", Embedding: []float32{0, 1}, Fingerprint: &structurallyNear, Hash: "hiring", CreatedAt: time.Now()},
		{ProjectID: "proj", Path: "projects/proj/sparse.md", Title: "Sparse match", Content: "sparse", Embedding: []float32{0, 1}, Fingerprint: &lowCoverage, Hash: "sparse", CreatedAt: time.Now()},
	}
	for _, memory := range memories {
		if err := idx.Upsert(ctx, memory, "test", "manual"); err != nil {
			t.Fatal(err)
		}
	}

	results, err := idx.SearchAnalogies(ctx, []float32{1, 0}, query, "proj", 10, 0.2, 0.8)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Title != "Hiring decision" {
		t.Fatalf("expected cross-domain structural match first, got %+v", results)
	}
	if results[0].SemanticScore >= results[1].SemanticScore {
		t.Fatalf("test setup did not create a semantic tradeoff: %+v", results)
	}
	if results[0].FingerprintScore <= results[1].FingerprintScore {
		t.Fatalf("fingerprint score did not distinguish structure: %+v", results)
	}
	if results[2].Title != "Sparse match" || results[2].AdjustedFingerprintScore >= results[0].AdjustedFingerprintScore {
		t.Fatalf("low-coverage match was not discounted: %+v", results)
	}
}
