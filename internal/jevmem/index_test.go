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
