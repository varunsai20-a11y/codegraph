package storage

import (
	"context"

	"codegraph/internal/models"
)

type RepositoryStore interface {
	CreateRepository(ctx context.Context, repo *models.Repository) error
	GetRepository(ctx context.Context, id string) (*models.Repository, error)
	ListRepositories(ctx context.Context) ([]*models.Repository, error)
	UpdateRepositoryStatus(ctx context.Context, id string, status models.RepositoryStatus) error
}

type IndexJobStore interface {
	CreateIndexJob(ctx context.Context, job *models.IndexJob) error
	GetIndexJob(ctx context.Context, id string) (*models.IndexJob, error)
	UpdateIndexJob(ctx context.Context, job *models.IndexJob) error
}

type ManifestStore interface {
	SaveManifestItems(ctx context.Context, repoID string, items []*models.FileManifestItem) error
	GetManifestForRepository(ctx context.Context, repoID string) ([]*models.FileManifestItem, error)
}

type CodeIntelligenceStore interface {
	SaveSymbols(ctx context.Context, repoID string, symbols []*models.Symbol) error
	GetSymbolsForRepository(ctx context.Context, repoID string) ([]*models.Symbol, error)
	SaveRelationships(ctx context.Context, repoID string, rels []*models.Relationship) error
	GetRelationshipsForRepository(ctx context.Context, repoID string) ([]*models.Relationship, error)
}

type GraphStore interface {
	SaveGraph(ctx context.Context, repoID string, nodes []*models.Node, edges []*models.Edge) error
	GetGraphForRepository(ctx context.Context, repoID string) ([]*models.Node, []*models.Edge, error)
}

type Storage interface {
	RepositoryStore
	IndexJobStore
	ManifestStore
	CodeIntelligenceStore
	GraphStore
	SaveIndexData(ctx context.Context, repoID string, manifests []*models.FileManifestItem, symbols []*models.Symbol, rels []*models.Relationship, nodes []*models.Node, edges []*models.Edge) error
	Close() error
}
