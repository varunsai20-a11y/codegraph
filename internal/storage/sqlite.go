package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"codegraph/internal/models"

	_ "modernc.org/sqlite"
)

type SQLiteStorage struct {
	db *sql.DB
}

func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	dsn := dbPath
	if !strings.Contains(dsn, "?") {
		dsn += "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	s := &SQLiteStorage{db: db}
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize sqlite schema: %w", err)
	}

	return s, nil
}

func (s *SQLiteStorage) initSchema() error {
	_, _ = s.db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`)

	schema := `
	CREATE TABLE IF NOT EXISTS repositories (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		source_type TEXT NOT NULL,
		source_url TEXT,
		local_path TEXT NOT NULL,
		default_branch TEXT,
		commit_sha TEXT,
		status TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS index_jobs (
		id TEXT PRIMARY KEY,
		repository_id TEXT NOT NULL,
		status TEXT NOT NULL,
		started_at DATETIME NOT NULL,
		completed_at DATETIME,
		files_discovered INTEGER NOT NULL DEFAULT 0,
		files_indexed INTEGER NOT NULL DEFAULT 0,
		files_skipped INTEGER NOT NULL DEFAULT 0,
		files_failed INTEGER NOT NULL DEFAULT 0,
		error TEXT,
		FOREIGN KEY(repository_id) REFERENCES repositories(id)
	);

	CREATE TABLE IF NOT EXISTS file_manifests (
		id TEXT PRIMARY KEY,
		repository_id TEXT NOT NULL,
		relative_path TEXT NOT NULL,
		language TEXT NOT NULL,
		extension TEXT NOT NULL,
		size INTEGER NOT NULL,
		sha256 TEXT NOT NULL,
		status TEXT NOT NULL,
		error_message TEXT,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY(repository_id) REFERENCES repositories(id),
		UNIQUE(repository_id, relative_path)
	);

	CREATE TABLE IF NOT EXISTS symbols (
		id TEXT PRIMARY KEY,
		repository_id TEXT NOT NULL,
		file_id TEXT NOT NULL,
		relative_path TEXT NOT NULL,
		name TEXT NOT NULL,
		qualified_name TEXT NOT NULL,
		kind TEXT NOT NULL,
		parent_id TEXT,
		start_line INTEGER NOT NULL,
		start_column INTEGER NOT NULL,
		end_line INTEGER NOT NULL,
		end_column INTEGER NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY(repository_id) REFERENCES repositories(id)
	);

	CREATE TABLE IF NOT EXISTS relationships (
		id TEXT PRIMARY KEY,
		repository_id TEXT NOT NULL,
		source_id TEXT NOT NULL,
		target_id TEXT NOT NULL,
		target_kind TEXT NOT NULL,
		type TEXT NOT NULL,
		status TEXT NOT NULL,
		file_id TEXT NOT NULL,
		start_line INTEGER NOT NULL,
		start_column INTEGER NOT NULL,
		end_line INTEGER NOT NULL,
		end_column INTEGER NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY(repository_id) REFERENCES repositories(id)
	);

	CREATE TABLE IF NOT EXISTS graph_nodes (
		id TEXT PRIMARY KEY,
		repository_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		label TEXT NOT NULL,
		qualified_name TEXT NOT NULL,
		file_id TEXT,
		relative_path TEXT,
		start_line INTEGER NOT NULL DEFAULT 0,
		start_column INTEGER NOT NULL DEFAULT 0,
		end_line INTEGER NOT NULL DEFAULT 0,
		end_column INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY(repository_id) REFERENCES repositories(id)
	);

	CREATE TABLE IF NOT EXISTS graph_edges (
		id TEXT PRIMARY KEY,
		repository_id TEXT NOT NULL,
		source_id TEXT NOT NULL,
		target_id TEXT NOT NULL,
		target_kind TEXT NOT NULL,
		kind TEXT NOT NULL,
		status TEXT NOT NULL,
		file_id TEXT NOT NULL,
		start_line INTEGER NOT NULL DEFAULT 0,
		start_column INTEGER NOT NULL DEFAULT 0,
		end_line INTEGER NOT NULL DEFAULT 0,
		end_column INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY(repository_id) REFERENCES repositories(id)
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

// RepositoryStore implementations

func (s *SQLiteStorage) CreateRepository(ctx context.Context, repo *models.Repository) error {
	query := `
	INSERT INTO repositories (id, name, source_type, source_url, local_path, default_branch, commit_sha, status, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.ExecContext(ctx, query,
		repo.ID, repo.Name, repo.SourceType, repo.SourceURL, repo.LocalPath,
		repo.DefaultBranch, repo.CommitSHA, repo.Status, repo.CreatedAt, repo.UpdatedAt,
	)
	return err
}

func (s *SQLiteStorage) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	query := `SELECT id, name, source_type, source_url, local_path, default_branch, commit_sha, status, created_at, updated_at FROM repositories WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, id)

	var repo models.Repository
	var srcUrl, defBranch, commitSha sql.NullString

	err := row.Scan(
		&repo.ID, &repo.Name, &repo.SourceType, &srcUrl, &repo.LocalPath,
		&defBranch, &commitSha, &repo.Status, &repo.CreatedAt, &repo.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("repository not found: %s", id)
		}
		return nil, err
	}

	repo.SourceURL = srcUrl.String
	repo.DefaultBranch = defBranch.String
	repo.CommitSHA = commitSha.String

	return &repo, nil
}

func (s *SQLiteStorage) ListRepositories(ctx context.Context) ([]*models.Repository, error) {
	query := `SELECT id, name, source_type, source_url, local_path, default_branch, commit_sha, status, created_at, updated_at FROM repositories ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*models.Repository
	for rows.Next() {
		var repo models.Repository
		var srcUrl, defBranch, commitSha sql.NullString
		if err := rows.Scan(
			&repo.ID, &repo.Name, &repo.SourceType, &srcUrl, &repo.LocalPath,
			&defBranch, &commitSha, &repo.Status, &repo.CreatedAt, &repo.UpdatedAt,
		); err != nil {
			return nil, err
		}
		repo.SourceURL = srcUrl.String
		repo.DefaultBranch = defBranch.String
		repo.CommitSHA = commitSha.String
		results = append(results, &repo)
	}
	return results, nil
}

func (s *SQLiteStorage) UpdateRepositoryStatus(ctx context.Context, id string, status models.RepositoryStatus) error {
	query := `UPDATE repositories SET status = ?, updated_at = ? WHERE id = ?`
	_, err := s.db.ExecContext(ctx, query, status, time.Now(), id)
	return err
}

// IndexJobStore implementations

func (s *SQLiteStorage) CreateIndexJob(ctx context.Context, job *models.IndexJob) error {
	query := `
	INSERT INTO index_jobs (id, repository_id, status, started_at, completed_at, files_discovered, files_indexed, files_skipped, files_failed, error)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.ExecContext(ctx, query,
		job.ID, job.RepositoryID, job.Status, job.StartedAt, job.CompletedAt,
		job.FilesDiscovered, job.FilesIndexed, job.FilesSkipped, job.FilesFailed, job.Error,
	)
	return err
}

func (s *SQLiteStorage) GetIndexJob(ctx context.Context, id string) (*models.IndexJob, error) {
	query := `SELECT id, repository_id, status, started_at, completed_at, files_discovered, files_indexed, files_skipped, files_failed, error FROM index_jobs WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, id)

	var job models.IndexJob
	var errStr sql.NullString
	var compAt sql.NullTime

	err := row.Scan(
		&job.ID, &job.RepositoryID, &job.Status, &job.StartedAt, &compAt,
		&job.FilesDiscovered, &job.FilesIndexed, &job.FilesSkipped, &job.FilesFailed, &errStr,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("index job not found: %s", id)
		}
		return nil, err
	}

	if compAt.Valid {
		job.CompletedAt = &compAt.Time
	}
	job.Error = errStr.String

	return &job, nil
}

func (s *SQLiteStorage) UpdateIndexJob(ctx context.Context, job *models.IndexJob) error {
	query := `
	UPDATE index_jobs SET status = ?, completed_at = ?, files_discovered = ?, files_indexed = ?, files_skipped = ?, files_failed = ?, error = ?
	WHERE id = ?
	`
	_, err := s.db.ExecContext(ctx, query,
		job.Status, job.CompletedAt, job.FilesDiscovered, job.FilesIndexed, job.FilesSkipped, job.FilesFailed, job.Error, job.ID,
	)
	return err
}

// ManifestStore implementations

func (s *SQLiteStorage) SaveManifestItems(ctx context.Context, repoID string, items []*models.FileManifestItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
	INSERT INTO file_manifests (id, repository_id, relative_path, language, extension, size, sha256, status, error_message, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(repository_id, relative_path) DO UPDATE SET
		language=excluded.language,
		extension=excluded.extension,
		size=excluded.size,
		sha256=excluded.sha256,
		status=excluded.status,
		error_message=excluded.error_message,
		updated_at=excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, item := range items {
		_, err := stmt.ExecContext(ctx,
			item.ID, repoID, item.RelativePath, item.Language, item.Extension,
			item.Size, item.SHA256, item.Status, item.ErrorMessage, item.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStorage) GetManifestForRepository(ctx context.Context, repoID string) ([]*models.FileManifestItem, error) {
	query := `SELECT id, repository_id, relative_path, language, extension, size, sha256, status, error_message, updated_at FROM file_manifests WHERE repository_id = ? ORDER BY relative_path ASC`
	rows, err := s.db.QueryContext(ctx, query, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*models.FileManifestItem
	for rows.Next() {
		var item models.FileManifestItem
		var errMsg sql.NullString
		if err := rows.Scan(
			&item.ID, &item.RepositoryID, &item.RelativePath, &item.Language, &item.Extension,
			&item.Size, &item.SHA256, &item.Status, &errMsg, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.ErrorMessage = errMsg.String
		results = append(results, &item)
	}
	return results, nil
}

// CodeIntelligenceStore implementations

func (s *SQLiteStorage) SaveSymbols(ctx context.Context, repoID string, symbols []*models.Symbol) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
	INSERT INTO symbols (id, repository_id, file_id, relative_path, name, qualified_name, kind, parent_id, start_line, start_column, end_line, end_column, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		name=excluded.name,
		qualified_name=excluded.qualified_name,
		kind=excluded.kind,
		parent_id=excluded.parent_id,
		start_line=excluded.start_line,
		start_column=excluded.start_column,
		end_line=excluded.end_line,
		end_column=excluded.end_column,
		updated_at=excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, sym := range symbols {
		var parentID sql.NullString
		if sym.ParentID != "" {
			parentID.String = sym.ParentID
			parentID.Valid = true
		}

		_, err := stmt.ExecContext(ctx,
			sym.ID, repoID, sym.FileID, sym.RelativePath, sym.Name, sym.QualifiedName,
			string(sym.Kind), parentID, sym.Location.StartLine, sym.Location.StartColumn,
			sym.Location.EndLine, sym.Location.EndColumn, sym.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStorage) GetSymbolsForRepository(ctx context.Context, repoID string) ([]*models.Symbol, error) {
	query := `SELECT id, repository_id, file_id, relative_path, name, qualified_name, kind, parent_id, start_line, start_column, end_line, end_column, updated_at FROM symbols WHERE repository_id = ? ORDER BY relative_path ASC, start_line ASC`
	rows, err := s.db.QueryContext(ctx, query, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*models.Symbol
	for rows.Next() {
		var sym models.Symbol
		var parentID sql.NullString
		var kindStr string
		if err := rows.Scan(
			&sym.ID, &sym.RepositoryID, &sym.FileID, &sym.RelativePath, &sym.Name, &sym.QualifiedName,
			&kindStr, &parentID, &sym.Location.StartLine, &sym.Location.StartColumn,
			&sym.Location.EndLine, &sym.Location.EndColumn, &sym.UpdatedAt,
		); err != nil {
			return nil, err
		}
		sym.Kind = models.SymbolKind(kindStr)
		sym.ParentID = parentID.String
		results = append(results, &sym)
	}
	return results, nil
}

func (s *SQLiteStorage) SaveRelationships(ctx context.Context, repoID string, rels []*models.Relationship) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
	INSERT INTO relationships (id, repository_id, source_id, target_id, target_kind, type, status, file_id, start_line, start_column, end_line, end_column, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		target_kind=excluded.target_kind,
		type=excluded.type,
		status=excluded.status,
		start_line=excluded.start_line,
		start_column=excluded.start_column,
		end_line=excluded.end_line,
		end_column=excluded.end_column,
		updated_at=excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, rel := range rels {
		_, err := stmt.ExecContext(ctx,
			rel.ID, repoID, rel.SourceID, rel.TargetID, string(rel.TargetKind),
			string(rel.Type), string(rel.Status), rel.FileID, rel.Location.StartLine,
			rel.Location.StartColumn, rel.Location.EndLine, rel.Location.EndColumn, rel.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStorage) GetRelationshipsForRepository(ctx context.Context, repoID string) ([]*models.Relationship, error) {
	query := `SELECT id, repository_id, source_id, target_id, target_kind, type, status, file_id, start_line, start_column, end_line, end_column, updated_at FROM relationships WHERE repository_id = ? ORDER BY file_id ASC, start_line ASC`
	rows, err := s.db.QueryContext(ctx, query, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*models.Relationship
	for rows.Next() {
		var rel models.Relationship
		var tkStr, typeStr, statusStr string
		if err := rows.Scan(
			&rel.ID, &rel.RepositoryID, &rel.SourceID, &rel.TargetID, &tkStr,
			&typeStr, &statusStr, &rel.FileID, &rel.Location.StartLine,
			&rel.Location.StartColumn, &rel.Location.EndLine, &rel.Location.EndColumn, &rel.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rel.TargetKind = models.TargetKind(tkStr)
		rel.Type = models.RelationType(typeStr)
		rel.Status = models.RelationStatus(statusStr)
		results = append(results, &rel)
	}
	return results, nil
}

// GraphStore implementations

func (s *SQLiteStorage) SaveGraph(ctx context.Context, repoID string, nodes []*models.Node, edges []*models.Edge) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Save Graph Nodes
	nodeStmt, err := tx.PrepareContext(ctx, `
	INSERT INTO graph_nodes (id, repository_id, kind, label, qualified_name, file_id, relative_path, start_line, start_column, end_line, end_column, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		label=excluded.label,
		qualified_name=excluded.qualified_name,
		file_id=excluded.file_id,
		relative_path=excluded.relative_path,
		start_line=excluded.start_line,
		start_column=excluded.start_column,
		end_line=excluded.end_line,
		end_column=excluded.end_column,
		updated_at=excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer nodeStmt.Close()

	for _, n := range nodes {
		_, err := nodeStmt.ExecContext(ctx,
			n.ID, repoID, string(n.Kind), n.Label, n.QualifiedName,
			n.FileID, n.RelativePath, n.Location.StartLine, n.Location.StartColumn,
			n.Location.EndLine, n.Location.EndColumn, n.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	// 2. Save Graph Edges
	edgeStmt, err := tx.PrepareContext(ctx, `
	INSERT INTO graph_edges (id, repository_id, source_id, target_id, target_kind, kind, status, file_id, start_line, start_column, end_line, end_column, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		target_kind=excluded.target_kind,
		kind=excluded.kind,
		status=excluded.status,
		start_line=excluded.start_line,
		start_column=excluded.start_column,
		end_line=excluded.end_line,
		end_column=excluded.end_column,
		updated_at=excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer edgeStmt.Close()

	for _, e := range edges {
		_, err := edgeStmt.ExecContext(ctx,
			e.ID, repoID, e.SourceID, e.TargetID, string(e.TargetKind),
			string(e.Kind), string(e.Status), e.FileID, e.Location.StartLine,
			e.Location.StartColumn, e.Location.EndLine, e.Location.EndColumn, e.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStorage) GetGraphForRepository(ctx context.Context, repoID string) ([]*models.Node, []*models.Edge, error) {
	// Query Nodes
	nodeQuery := `SELECT id, repository_id, kind, label, qualified_name, file_id, relative_path, start_line, start_column, end_line, end_column, updated_at FROM graph_nodes WHERE repository_id = ? ORDER BY id ASC`
	nodeRows, err := s.db.QueryContext(ctx, nodeQuery, repoID)
	if err != nil {
		return nil, nil, err
	}
	defer nodeRows.Close()

	var nodes []*models.Node
	for nodeRows.Next() {
		var n models.Node
		var fileID, relPath sql.NullString
		var kindStr string
		if err := nodeRows.Scan(
			&n.ID, &n.RepositoryID, &kindStr, &n.Label, &n.QualifiedName,
			&fileID, &relPath, &n.Location.StartLine, &n.Location.StartColumn,
			&n.Location.EndLine, &n.Location.EndColumn, &n.UpdatedAt,
		); err != nil {
			return nil, nil, err
		}
		n.Kind = models.NodeKind(kindStr)
		n.FileID = fileID.String
		n.RelativePath = relPath.String
		nodes = append(nodes, &n)
	}

	// Query Edges
	edgeQuery := `SELECT id, repository_id, source_id, target_id, target_kind, kind, status, file_id, start_line, start_column, end_line, end_column, updated_at FROM graph_edges WHERE repository_id = ? ORDER BY id ASC`
	edgeRows, err := s.db.QueryContext(ctx, edgeQuery, repoID)
	if err != nil {
		return nil, nil, err
	}
	defer edgeRows.Close()

	var edges []*models.Edge
	for edgeRows.Next() {
		var e models.Edge
		var tkStr, kindStr, statusStr string
		if err := edgeRows.Scan(
			&e.ID, &e.RepositoryID, &e.SourceID, &e.TargetID, &tkStr,
			&kindStr, &statusStr, &e.FileID, &e.Location.StartLine,
			&e.Location.StartColumn, &e.Location.EndLine, &e.Location.EndColumn, &e.UpdatedAt,
		); err != nil {
			return nil, nil, err
		}
		e.TargetKind = models.TargetKind(tkStr)
		e.Kind = models.EdgeKind(kindStr)
		e.Status = models.RelationStatus(statusStr)
		edges = append(edges, &e)
	}

	return nodes, edges, nil
}

// SaveIndexData atomically persists all index artifacts (manifests, symbols, relationships, graph_nodes, graph_edges) for a repository in a single transaction.
func (s *SQLiteStorage) SaveIndexData(
	ctx context.Context,
	repoID string,
	manifests []*models.FileManifestItem,
	symbols []*models.Symbol,
	rels []*models.Relationship,
	nodes []*models.Node,
	edges []*models.Edge,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	delQueries := []string{
		`DELETE FROM file_manifests WHERE repository_id = ?`,
		`DELETE FROM symbols WHERE repository_id = ?`,
		`DELETE FROM relationships WHERE repository_id = ?`,
		`DELETE FROM graph_nodes WHERE repository_id = ?`,
		`DELETE FROM graph_edges WHERE repository_id = ?`,
	}
	for _, q := range delQueries {
		if _, err := tx.ExecContext(ctx, q, repoID); err != nil {
			return err
		}
	}

	manifestStmt, err := tx.PrepareContext(ctx, `
	INSERT INTO file_manifests (id, repository_id, relative_path, language, extension, size, sha256, status, error_message, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(repository_id, relative_path) DO UPDATE SET
		language=excluded.language,
		extension=excluded.extension,
		size=excluded.size,
		sha256=excluded.sha256,
		status=excluded.status,
		error_message=excluded.error_message,
		updated_at=excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer manifestStmt.Close()

	for _, item := range manifests {
		_, err := manifestStmt.ExecContext(ctx,
			item.ID, repoID, item.RelativePath, item.Language, item.Extension,
			item.Size, item.SHA256, item.Status, item.ErrorMessage, item.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	symStmt, err := tx.PrepareContext(ctx, `
	INSERT INTO symbols (id, repository_id, file_id, relative_path, name, qualified_name, kind, parent_id, start_line, start_column, end_line, end_column, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		name=excluded.name,
		qualified_name=excluded.qualified_name,
		kind=excluded.kind,
		parent_id=excluded.parent_id,
		start_line=excluded.start_line,
		start_column=excluded.start_column,
		end_line=excluded.end_line,
		end_column=excluded.end_column,
		updated_at=excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer symStmt.Close()

	for _, sym := range symbols {
		var parentID sql.NullString
		if sym.ParentID != "" {
			parentID.String = sym.ParentID
			parentID.Valid = true
		}
		_, err := symStmt.ExecContext(ctx,
			sym.ID, repoID, sym.FileID, sym.RelativePath, sym.Name, sym.QualifiedName,
			string(sym.Kind), parentID, sym.Location.StartLine, sym.Location.StartColumn,
			sym.Location.EndLine, sym.Location.EndColumn, sym.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	relStmt, err := tx.PrepareContext(ctx, `
	INSERT INTO relationships (id, repository_id, source_id, target_id, target_kind, type, status, file_id, start_line, start_column, end_line, end_column, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		target_kind=excluded.target_kind,
		type=excluded.type,
		status=excluded.status,
		start_line=excluded.start_line,
		start_column=excluded.start_column,
		end_line=excluded.end_line,
		end_column=excluded.end_column,
		updated_at=excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer relStmt.Close()

	for _, rel := range rels {
		_, err := relStmt.ExecContext(ctx,
			rel.ID, repoID, rel.SourceID, rel.TargetID, string(rel.TargetKind),
			string(rel.Type), string(rel.Status), rel.FileID, rel.Location.StartLine,
			rel.Location.StartColumn, rel.Location.EndLine, rel.Location.EndColumn, rel.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	nodeStmt, err := tx.PrepareContext(ctx, `
	INSERT INTO graph_nodes (id, repository_id, kind, label, qualified_name, file_id, relative_path, start_line, start_column, end_line, end_column, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		label=excluded.label,
		qualified_name=excluded.qualified_name,
		file_id=excluded.file_id,
		relative_path=excluded.relative_path,
		start_line=excluded.start_line,
		start_column=excluded.start_column,
		end_line=excluded.end_line,
		end_column=excluded.end_column,
		updated_at=excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer nodeStmt.Close()

	for _, n := range nodes {
		_, err := nodeStmt.ExecContext(ctx,
			n.ID, repoID, string(n.Kind), n.Label, n.QualifiedName,
			n.FileID, n.RelativePath, n.Location.StartLine, n.Location.StartColumn,
			n.Location.EndLine, n.Location.EndColumn, n.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	edgeStmt, err := tx.PrepareContext(ctx, `
	INSERT INTO graph_edges (id, repository_id, source_id, target_id, target_kind, kind, status, file_id, start_line, start_column, end_line, end_column, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		target_kind=excluded.target_kind,
		kind=excluded.kind,
		status=excluded.status,
		start_line=excluded.start_line,
		start_column=excluded.start_column,
		end_line=excluded.end_line,
		end_column=excluded.end_column,
		updated_at=excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer edgeStmt.Close()

	for _, e := range edges {
		_, err := edgeStmt.ExecContext(ctx,
			e.ID, repoID, e.SourceID, e.TargetID, string(e.TargetKind),
			string(e.Kind), string(e.Status), e.FileID, e.Location.StartLine,
			e.Location.StartColumn, e.Location.EndLine, e.Location.EndColumn, e.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
