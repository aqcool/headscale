package biz

import (
	"context"
	"errors"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// PreAuthKey is a PreAuthKey model.
type PreAuthKey struct {
	ID        int
	Key       string
	Reusable  bool
	Ephemeral bool
	UsedCount int
	UserID    uint
	Tags      []string
	Expiry    *time.Time
	CreatedAt time.Time
}

// PreAuthKeyRepo is a PreAuthKey repo.
type PreAuthKeyRepo interface {
	Save(ctx context.Context, key *PreAuthKey) (*PreAuthKey, error)
	FindByKey(ctx context.Context, key string) (*PreAuthKey, error)
	FindByID(ctx context.Context, id int) (*PreAuthKey, error)
	ListByUser(ctx context.Context, userID uint) ([]*PreAuthKey, error)
	ListAll(ctx context.Context) ([]*PreAuthKey, error)
	Delete(ctx context.Context, id int) error
	Expire(ctx context.Context, id int) error
	IncrementUsage(ctx context.Context, key string) error
}

// PreAuthKeyUsecase is a PreAuthKey usecase.
type PreAuthKeyUsecase struct {
	repo PreAuthKeyRepo
	log  *log.Helper
}

// NewPreAuthKeyUsecase creates a new PreAuthKey usecase.
func NewPreAuthKeyUsecase(repo PreAuthKeyRepo, logger log.Logger) *PreAuthKeyUsecase {
	return &PreAuthKeyUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

// CreatePreAuthKey creates a PreAuthKey.
func (uc *PreAuthKeyUsecase) CreatePreAuthKey(ctx context.Context, key *PreAuthKey) (*PreAuthKey, error) {
	return uc.repo.Save(ctx, key)
}

// GetPreAuthKey gets a PreAuthKey by key string.
func (uc *PreAuthKeyUsecase) GetPreAuthKey(ctx context.Context, key string) (*PreAuthKey, error) {
	return uc.repo.FindByKey(ctx, key)
}

// GetPreAuthKeyByID gets a PreAuthKey by ID.
func (uc *PreAuthKeyUsecase) GetPreAuthKeyByID(ctx context.Context, id int) (*PreAuthKey, error) {
	return uc.repo.FindByID(ctx, id)
}

// ListPreAuthKeys lists PreAuthKeys by user ID. If userID is 0, lists all.
func (uc *PreAuthKeyUsecase) ListPreAuthKeys(ctx context.Context, userID uint) ([]*PreAuthKey, error) {
	if userID == 0 {
		return uc.repo.ListAll(ctx)
	}
	return uc.repo.ListByUser(ctx, userID)
}

// DeletePreAuthKey deletes a PreAuthKey.
func (uc *PreAuthKeyUsecase) DeletePreAuthKey(ctx context.Context, id int) error {
	return uc.repo.Delete(ctx, id)
}

// ExpirePreAuthKey expires a PreAuthKey.
func (uc *PreAuthKeyUsecase) ExpirePreAuthKey(ctx context.Context, id int) error {
	return uc.repo.Expire(ctx, id)
}

// UsePreAuthKey increments the usage count for a key.
func (uc *PreAuthKeyUsecase) UsePreAuthKey(ctx context.Context, key string) error {
	return uc.repo.IncrementUsage(ctx, key)
}

// ValidatePreAuthKey validates a pre-auth key and returns it if valid.
func (uc *PreAuthKeyUsecase) ValidatePreAuthKey(ctx context.Context, key string) (*PreAuthKey, error) {
	pak, err := uc.repo.FindByKey(ctx, key)
	if err != nil {
		return nil, err
	}

	// Check if expired
	if pak.Expiry != nil && time.Now().After(*pak.Expiry) {
		return nil, errors.New("pre-auth key expired")
	}

	// Check if reusable or not already used
	if !pak.Reusable && pak.UsedCount > 0 {
		return nil, errors.New("pre-auth key already used")
	}

	return pak, nil
}
