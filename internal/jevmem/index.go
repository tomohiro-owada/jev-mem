package jevmem

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

type Index struct {
	db *sql.DB
}

func OpenIndex(path string) (*Index, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	idx := &Index{db: db}
	if err := idx.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return idx, nil
}

func (i *Index) Close() error {
	if i == nil || i.db == nil {
		return nil
	}
	return i.db.Close()
}

func (i *Index) init() error {
	_, err := i.db.Exec(`
CREATE TABLE IF NOT EXISTS memories (
  path TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  embedding TEXT NOT NULL,
  embedding_status TEXT NOT NULL,
  embedding_error TEXT,
  embedding_provider TEXT NOT NULL,
  embedding_model TEXT NOT NULL,
  embedding_dim INTEGER NOT NULL,
	  fingerprint TEXT,
	  fingerprint_status TEXT NOT NULL DEFAULT 'missing',
	  fingerprint_schema INTEGER,
	  fingerprint_extractor TEXT,
	  fingerprint_extractor_version TEXT,
  content_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  indexed_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS memories_project_id_idx ON memories(project_id);
CREATE INDEX IF NOT EXISTS memories_content_hash_idx ON memories(content_hash);
CREATE INDEX IF NOT EXISTS memories_fingerprint_status_idx ON memories(fingerprint_status);
`)
	if err != nil {
		return err
	}
	columns := []struct {
		name       string
		definition string
	}{
		{"fingerprint", "TEXT"},
		{"fingerprint_status", "TEXT NOT NULL DEFAULT 'missing'"},
		{"fingerprint_schema", "INTEGER"},
		{"fingerprint_extractor", "TEXT"},
		{"fingerprint_extractor_version", "TEXT"},
	}
	for _, column := range columns {
		if err := i.ensureColumn("memories", column.name, column.definition); err != nil {
			return err
		}
	}
	_, err = i.db.Exec(`CREATE INDEX IF NOT EXISTS memories_fingerprint_status_idx ON memories(fingerprint_status)`)
	return err
}

func (i *Index) ensureColumn(table, name, definition string) error {
	rows, err := i.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var columnName, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if columnName == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = i.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + name + ` ` + definition)
	return err
}

func (i *Index) Upsert(ctx context.Context, mem Memory, provider, model string) error {
	if len(mem.Embedding) == 0 {
		return errors.New("embedding is required")
	}
	emb, err := json.Marshal(mem.Embedding)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if mem.IndexedAt.IsZero() {
		mem.IndexedAt = now
	}
	var fingerprintJSON any
	fingerprintStatus := "missing"
	var fingerprintSchema any
	var fingerprintExtractor any
	var fingerprintExtractorVersion any
	if mem.Fingerprint != nil {
		if err := ValidateFingerprint(*mem.Fingerprint); err != nil {
			return fmt.Errorf("invalid fingerprint: %w", err)
		}
		fingerprint, err := json.Marshal(mem.Fingerprint)
		if err != nil {
			return err
		}
		fingerprintJSON = string(fingerprint)
		fingerprintStatus = "ready"
		fingerprintSchema = mem.Fingerprint.SchemaVersion
		fingerprintExtractor = mem.Fingerprint.Extractor
		fingerprintExtractorVersion = mem.Fingerprint.ExtractorVersion
	}
	_, err = i.db.ExecContext(ctx, `
INSERT INTO memories(path, project_id, title, content, embedding, embedding_status, embedding_error, embedding_provider, embedding_model, embedding_dim, fingerprint, fingerprint_status, fingerprint_schema, fingerprint_extractor, fingerprint_extractor_version, content_hash, created_at, indexed_at)
VALUES(?, ?, ?, ?, ?, 'ready', NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(path) DO UPDATE SET
  project_id=excluded.project_id,
  title=excluded.title,
  content=excluded.content,
  embedding=excluded.embedding,
  embedding_status='ready',
  embedding_error=NULL,
  embedding_provider=excluded.embedding_provider,
  embedding_model=excluded.embedding_model,
  embedding_dim=excluded.embedding_dim,
	  fingerprint=excluded.fingerprint,
	  fingerprint_status=excluded.fingerprint_status,
	  fingerprint_schema=excluded.fingerprint_schema,
	  fingerprint_extractor=excluded.fingerprint_extractor,
	  fingerprint_extractor_version=excluded.fingerprint_extractor_version,
  content_hash=excluded.content_hash,
  created_at=excluded.created_at,
  indexed_at=excluded.indexed_at
`, mem.Path, mem.ProjectID, mem.Title, mem.Content, string(emb), provider, model, len(mem.Embedding), fingerprintJSON, fingerprintStatus, fingerprintSchema, fingerprintExtractor, fingerprintExtractorVersion, mem.Hash, mem.CreatedAt.UTC().Format(time.RFC3339), mem.IndexedAt.UTC().Format(time.RFC3339))
	return err
}

func (i *Index) FindByPath(ctx context.Context, path string) (string, bool, error) {
	var hash string
	err := i.db.QueryRowContext(ctx, `SELECT content_hash FROM memories WHERE path = ?`, path).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return hash, true, nil
}

func (i *Index) Search(ctx context.Context, query []float32, projectID string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := i.db.QueryContext(ctx, `SELECT project_id, path, title, content, embedding FROM memories WHERE (? = '' OR project_id = ?)`, projectID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []SearchResult
	for rows.Next() {
		var project, path, title, content, embJSON string
		if err := rows.Scan(&project, &path, &title, &content, &embJSON); err != nil {
			return nil, err
		}
		var emb []float32
		if err := json.Unmarshal([]byte(embJSON), &emb); err != nil {
			continue
		}
		results = append(results, SearchResult{
			ProjectID: project,
			Path:      path,
			Title:     title,
			Content:   content,
			Score:     Cosine(query, emb),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(results, func(a, b int) bool {
		return results[a].Score > results[b].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (i *Index) SearchFingerprint(ctx context.Context, query Fingerprint, projectID string, limit int) ([]FingerprintSearchResult, error) {
	if err := ValidateFingerprint(query); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}
	rows, err := i.db.QueryContext(ctx, `SELECT project_id, path, title, content, fingerprint FROM memories WHERE fingerprint_status = 'ready' AND (? = '' OR project_id = ?)`, projectID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []FingerprintSearchResult{}
	for rows.Next() {
		var project, path, title, content, raw string
		if err := rows.Scan(&project, &path, &title, &content, &raw); err != nil {
			return nil, err
		}
		var candidate Fingerprint
		if err := json.Unmarshal([]byte(raw), &candidate); err != nil {
			continue
		}
		similarity, err := CompareFingerprints(query, candidate)
		if err != nil {
			continue
		}
		results = append(results, FingerprintSearchResult{
			ProjectID:  project,
			Path:       path,
			Title:      title,
			Content:    content,
			Similarity: similarity,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(results, func(a, b int) bool {
		if results[a].Similarity.Score == results[b].Similarity.Score {
			return results[a].Similarity.Coverage > results[b].Similarity.Coverage
		}
		return results[a].Similarity.Score > results[b].Similarity.Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (i *Index) Count(ctx context.Context) (int, error) {
	var count int
	err := i.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories`).Scan(&count)
	return count, err
}

func (i *Index) FailedEmbeddingCount(ctx context.Context) (int, error) {
	var count int
	err := i.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories WHERE embedding_status != 'ready'`).Scan(&count)
	return count, err
}
