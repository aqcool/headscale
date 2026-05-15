package state

import (
	"context"
	"time"

	"github.com/juanfont/headscale-v2/internal/db"
	"github.com/juanfont/headscale-v2/internal/types"
	"gorm.io/gorm"
)

type StateRepository interface {
	PersistNode(node *types.Node, columns []string, omitFields []string) error
	DeleteNode(node *types.Node) error
	SaveNode(node *types.Node) error
	GetUserByID(id types.UserID) (*types.User, error)
	GetUserByName(name string) (*types.User, error)
	ListUsers() ([]types.User, error)
	UpdateUser(userID types.UserID, updateFn func(*types.User) error) (*types.User, error)
	GetPreAuthKey(key string) (*types.PreAuthKey, error)
	UsePreAuthKey(pak *types.PreAuthKey) error
	ListPreAuthKeys() ([]types.PreAuthKey, error)
	DeletePreAuthKey(id uint64) error
	GetPolicy() (*types.Policy, error)
	CreateAPIKey(expiration *time.Time) (string, *types.APIKey, error)
	GetAPIKey(prefix string) (*types.APIKey, error)
	GetAPIKeyByID(id uint64) (*types.APIKey, error)
	ExpireAPIKey(key *types.APIKey) error
	ListAPIKeys() ([]types.APIKey, error)
	DestroyAPIKey(key types.APIKey) error
	PingDB(ctx context.Context) error
}

type GormStateRepository struct {
	hsdb *db.HSDatabase
}

func NewGormStateRepository(hsdb *db.HSDatabase) StateRepository {
	return &GormStateRepository{hsdb: hsdb}
}

func (r *GormStateRepository) PersistNode(node *types.Node, columns []string, omitFields []string) error {
	return r.hsdb.Write(func(tx *gorm.DB) error {
		query := tx.Model(node)
		if len(columns) > 0 {
			query = query.Select(columns)
		}
		if len(omitFields) > 0 {
			query = query.Omit(omitFields...)
		}
		return query.Updates(node).Error
	})
}

func (r *GormStateRepository) DeleteNode(node *types.Node) error {
	return r.hsdb.Write(func(tx *gorm.DB) error {
		return tx.Delete(node).Error
	})
}

func (r *GormStateRepository) SaveNode(node *types.Node) error {
	return r.hsdb.Write(func(tx *gorm.DB) error {
		return tx.Save(node).Error
	})
}

func (r *GormStateRepository) GetUserByID(id types.UserID) (*types.User, error) {
	return r.hsdb.GetUserByID(id)
}

func (r *GormStateRepository) GetUserByName(name string) (*types.User, error) {
	return r.hsdb.GetUserByName(name)
}

func (r *GormStateRepository) ListUsers() ([]types.User, error) {
	return r.hsdb.ListUsers()
}

func (r *GormStateRepository) GetPreAuthKey(key string) (*types.PreAuthKey, error) {
	return r.hsdb.GetPreAuthKey(key)
}

func (r *GormStateRepository) UsePreAuthKey(pak *types.PreAuthKey) error {
	return r.hsdb.Write(func(tx *gorm.DB) error {
		return db.UsePreAuthKey(tx, pak)
	})
}

func (r *GormStateRepository) PingDB(ctx context.Context) error {
	return r.hsdb.PingDB(ctx)
}

func (r *GormStateRepository) UpdateUser(userID types.UserID, updateFn func(*types.User) error) (*types.User, error) {
	return r.hsdb.UpdateUser(userID, updateFn)
}

func (r *GormStateRepository) ListPreAuthKeys() ([]types.PreAuthKey, error) {
	return r.hsdb.ListPreAuthKeys()
}

func (r *GormStateRepository) DeletePreAuthKey(id uint64) error {
	return r.hsdb.DeletePreAuthKey(id)
}

func (r *GormStateRepository) GetPolicy() (*types.Policy, error) {
	return r.hsdb.GetPolicy()
}

func (r *GormStateRepository) CreateAPIKey(expiration *time.Time) (string, *types.APIKey, error) {
	return r.hsdb.CreateAPIKey(expiration)
}

func (r *GormStateRepository) GetAPIKey(prefix string) (*types.APIKey, error) {
	return r.hsdb.GetAPIKey(prefix)
}

func (r *GormStateRepository) GetAPIKeyByID(id uint64) (*types.APIKey, error) {
	return r.hsdb.GetAPIKeyByID(id)
}

func (r *GormStateRepository) ExpireAPIKey(key *types.APIKey) error {
	return r.hsdb.ExpireAPIKey(key)
}

func (r *GormStateRepository) ListAPIKeys() ([]types.APIKey, error) {
	return r.hsdb.ListAPIKeys()
}

func (r *GormStateRepository) DestroyAPIKey(key types.APIKey) error {
	return r.hsdb.DestroyAPIKey(key)
}
