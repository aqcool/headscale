package data

import (
	"context"
	"time"

	"github.com/juanfont/headscale-v2/ent"
	"github.com/juanfont/headscale-v2/ent/apikey"
	"github.com/juanfont/headscale-v2/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
)

type apiKeyRepo struct {
	data *Data
	log  *log.Helper
}

// NewAPIKeyRepo new a apiKey repo.
func NewAPIKeyRepo(data *Data, logger log.Logger) biz.APIKeyRepo {
	return &apiKeyRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

func (r *apiKeyRepo) Save(ctx context.Context, k *biz.APIKey) (*biz.APIKey, error) {
	query := r.data.db.APIKey.Create().
		SetPrefix(k.Prefix).
		SetKey(k.Key)

	if k.Expiry != nil {
		query.SetExpiration(*k.Expiry)
	}

	po, err := query.Save(ctx)
	if err != nil {
		return nil, err
	}
	return entAPIKeyToBiz(po), nil
}

func (r *apiKeyRepo) FindByPrefix(ctx context.Context, prefix string) (*biz.APIKey, error) {
	po, err := r.data.db.APIKey.Query().Where(apikey.Prefix(prefix)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return entAPIKeyToBiz(po), nil
}

func (r *apiKeyRepo) FindByID(ctx context.Context, id int) (*biz.APIKey, error) {
	po, err := r.data.db.APIKey.Query().Where(apikey.ID(id)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return entAPIKeyToBiz(po), nil
}

func (r *apiKeyRepo) ListAll(ctx context.Context) ([]*biz.APIKey, error) {
	pos, err := r.data.db.APIKey.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]*biz.APIKey, 0, len(pos))
	for _, po := range pos {
		keys = append(keys, entAPIKeyToBiz(po))
	}
	return keys, nil
}

func (r *apiKeyRepo) Delete(ctx context.Context, prefix string) error {
	_, err := r.data.db.APIKey.Delete().Where(apikey.Prefix(prefix)).Exec(ctx)
	return err
}

func (r *apiKeyRepo) Expire(ctx context.Context, prefix string) error {
	now := time.Now()
	return r.data.db.APIKey.Update().
		Where(apikey.Prefix(prefix)).
		SetExpiration(now).
		Exec(ctx)
}

func entAPIKeyToBiz(po *ent.APIKey) *biz.APIKey {
	k := &biz.APIKey{
		ID:        po.ID,
		Prefix:    po.Prefix,
		Key:       po.Key,
		CreatedAt: po.CreatedAt,
	}
	if po.Expiration != nil {
		k.Expiry = po.Expiration
	}
	return k
}