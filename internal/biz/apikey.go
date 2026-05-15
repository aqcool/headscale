package biz

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// APIKey is a APIKey model.
type APIKey struct {
	ID        int
	Prefix    string
	Key       string
	CreatedAt time.Time
	Expiry    *time.Time
}

// APIKeyRepo is a APIKey repo.
type APIKeyRepo interface {
	Save(ctx context.Context, key *APIKey) (*APIKey, error)
	FindByPrefix(ctx context.Context, prefix string) (*APIKey, error)
	FindByID(ctx context.Context, id int) (*APIKey, error)
	ListAll(ctx context.Context) ([]*APIKey, error)
	Delete(ctx context.Context, prefix string) error
	Expire(ctx context.Context, prefix string) error
}

// APIKeyUsecase is a APIKey usecase.
type APIKeyUsecase struct {
	repo APIKeyRepo
	log  *log.Helper
}

// NewAPIKeyUsecase creates a new APIKey usecase.
func NewAPIKeyUsecase(repo APIKeyRepo, logger log.Logger) *APIKeyUsecase {
	return &APIKeyUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

// CreateAPIKey creates a APIKey.
func (uc *APIKeyUsecase) CreateAPIKey(ctx context.Context, key *APIKey) (*APIKey, error) {
	return uc.repo.Save(ctx, key)
}

// GetAPIKey gets a APIKey by prefix.
func (uc *APIKeyUsecase) GetAPIKey(ctx context.Context, prefix string) (*APIKey, error) {
	return uc.repo.FindByPrefix(ctx, prefix)
}

// GetAPIKeyByID gets a APIKey by ID.
func (uc *APIKeyUsecase) GetAPIKeyByID(ctx context.Context, id int) (*APIKey, error) {
	return uc.repo.FindByID(ctx, id)
}

// GetAPIKeyByPrefix gets a APIKey by prefix.
func (uc *APIKeyUsecase) GetAPIKeyByPrefix(ctx context.Context, prefix string) (*APIKey, error) {
	return uc.repo.FindByPrefix(ctx, prefix)
}

// ListAPIKeys lists all APIKeys.
func (uc *APIKeyUsecase) ListAPIKeys(ctx context.Context) ([]*APIKey, error) {
	return uc.repo.ListAll(ctx)
}

// DeleteAPIKey deletes a APIKey.
func (uc *APIKeyUsecase) DeleteAPIKey(ctx context.Context, prefix string) error {
	return uc.repo.Delete(ctx, prefix)
}

// ExpireAPIKey expires a APIKey.
func (uc *APIKeyUsecase) ExpireAPIKey(ctx context.Context, prefix string) error {
	return uc.repo.Expire(ctx, prefix)
}
