package vector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"codegraph/internal/models"

	_ "modernc.org/sqlite"
)

// SQLiteSemanticStore implements SemanticStore using SQLite for embedded pure-Go vector persistence.
type SQLiteSemanticStore struct {
	db      *sql.DB
	ownedDB bool
}

func NewSQLiteSemanticStore(db *sql.DB) (*SQLiteSemanticStore, error) {
	s := &SQLiteSemanticStore{db: db, ownedDB: false}
	if err := s.initSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

func NewSQLiteSemanticStorePath(dbPath string) (*SQLiteSemanticStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database for vector store: %w", err)
	}
	s := &SQLiteSemanticStore{db: db, ownedDB: true}
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteSemanticStore) initSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS vector_embeddings (
		id TEXT PRIMARY KEY,
		repository_id TEXT NOT NULL,
		chunk_id TEXT NOT NULL,
		symbol_id TEXT,
		relative_path TEXT NOT NULL,
		start_line INTEGER NOT NULL,
		end_line INTEGER NOT NULL,
		content_hash TEXT NOT NULL,
		content TEXT NOT NULL,
		embedding_model TEXT NOT NULL,
		embedding_dimension INTEGER NOT NULL,
		embedding_version TEXT NOT NULL,
		embedding_json TEXT NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_vector_repo ON vector_embeddings(repository_id);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *SQLiteSemanticStore) SaveEmbeddings(ctx context.Context, scope models.RepositoryScope, records []*VectorRecord) error {
	if scope.RepositoryID == "" {
		return models.ErrInvalidRepositoryScope
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO vector_embeddings (
			id, repository_id, chunk_id, symbol_id, relative_path,
			start_line, end_line, content_hash, content,
			embedding_model, embedding_dimension, embedding_version,
			embedding_json, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range records {
		if r.RepositoryID != scope.RepositoryID {
			return fmt.Errorf("%w: record repo '%s' != scope repo '%s'", models.ErrRepositoryMismatch, r.RepositoryID, scope.RepositoryID)
		}

		// Compute logical identity derived from (repoID, chunkID, model, version)
		// Re-indexing chunkID replaces previous content_hash vector rather than accumulating stale rows.
		if r.ID == "" {
			r.ID = ComputeVectorID(r.RepositoryID, r.ChunkID, r.EmbeddingModel, r.EmbeddingVersion)
		}
		if r.UpdatedAt.IsZero() {
			r.UpdatedAt = time.Now()
		}

		vecJSON, err := json.Marshal(r.Vector)
		if err != nil {
			return fmt.Errorf("failed to marshal embedding vector: %w", err)
		}

		_, err = stmt.ExecContext(ctx,
			r.ID, r.RepositoryID, r.ChunkID, r.SymbolID, r.RelativePath,
			r.Location.StartLine, r.Location.EndLine, r.ContentHash, r.Content,
			r.EmbeddingModel, r.EmbeddingDimension, r.EmbeddingVersion,
			string(vecJSON), r.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert vector embedding record: %w", err)
		}
	}

	return tx.Commit()
}

func (s *SQLiteSemanticStore) Search(
	ctx context.Context,
	scope models.RepositoryScope,
	queryVector []float32,
	modelName string,
	version string,
	limit int,
	minSimilarity float64,
) ([]*VectorSearchResult, error) {
	if scope.RepositoryID == "" {
		return nil, models.ErrInvalidRepositoryScope
	}
	if len(queryVector) == 0 {
		return nil, ErrEmptyVector
	}
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, repository_id, chunk_id, symbol_id, relative_path,
		       start_line, end_line, content_hash, content,
		       embedding_model, embedding_dimension, embedding_version,
		       embedding_json, updated_at
		FROM vector_embeddings
		WHERE repository_id = ?
	`, scope.RepositoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*VectorSearchResult

	for rows.Next() {
		var r VectorRecord
		var vecJSON string

		err := rows.Scan(
			&r.ID, &r.RepositoryID, &r.ChunkID, &r.SymbolID, &r.RelativePath,
			&r.Location.StartLine, &r.Location.EndLine, &r.ContentHash, &r.Content,
			&r.EmbeddingModel, &r.EmbeddingDimension, &r.EmbeddingVersion,
			&vecJSON, &r.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		// Verify embedding model & version match to prevent skew
		if r.EmbeddingModel != modelName || r.EmbeddingVersion != version {
			continue
		}

		if err := json.Unmarshal([]byte(vecJSON), &r.Vector); err != nil {
			continue
		}

		score, err := CosineSimilarity(queryVector, r.Vector)
		if err != nil {
			continue
		}

		// Optional minSimilarity threshold check
		if score < minSimilarity {
			continue
		}

		results = append(results, &VectorSearchResult{
			Record: &r,
			Score:  score,
		})
	}

	// Deterministic Sort: Primary = Score desc, Secondary = Record.ID asc
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Record.ID < results[j].Record.ID
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func (s *SQLiteSemanticStore) DeleteRepositoryEmbeddings(ctx context.Context, scope models.RepositoryScope) error {
	if scope.RepositoryID == "" {
		return models.ErrInvalidRepositoryScope
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM vector_embeddings WHERE repository_id = ?", scope.RepositoryID)
	return err
}

func (s *SQLiteSemanticStore) Close() error {
	if s.ownedDB && s.db != nil {
		return s.db.Close()
	}
	return nil
}
