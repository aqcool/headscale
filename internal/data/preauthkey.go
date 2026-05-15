package data

import (
	"context"
	"time"

	"github.com/juanfont/headscale-v2/ent"
	"github.com/juanfont/headscale-v2/ent/preauthkey"
	"github.com/juanfont/headscale-v2/ent/user"
	"github.com/juanfont/headscale-v2/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
)

type preAuthKeyRepo struct {
	data *Data
	log  *log.Helper
}

// NewPreAuthKeyRepo new a preAuthKey repo.
func NewPreAuthKeyRepo(data *Data, logger log.Logger) biz.PreAuthKeyRepo {
	return &preAuthKeyRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

func (r *preAuthKeyRepo) Save(ctx context.Context, k *biz.PreAuthKey) (*biz.PreAuthKey, error) {
	query := r.data.db.PreAuthKey.Create().
		SetKey(k.Key).
		SetReusable(k.Reusable).
		SetEphemeral(k.Ephemeral).
		SetUsedCount(k.UsedCount)

	if k.Expiry != nil {
		query.SetExpiration(*k.Expiry)
	}
	if k.UserID != 0 {
		query.SetUserID(int(k.UserID))
	}
	if len(k.Tags) > 0 {
		query.SetACLTags(k.Tags)
	}

	po, err := query.Save(ctx)
	if err != nil {
		return nil, err
	}
	return entPreAuthKeyToBiz(po), nil
}

func (r *preAuthKeyRepo) FindByKey(ctx context.Context, key string) (*biz.PreAuthKey, error) {
	po, err := r.data.db.PreAuthKey.Query().
		Where(preauthkey.Key(key)).
		WithUser().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return entPreAuthKeyToBiz(po), nil
}

func (r *preAuthKeyRepo) FindByID(ctx context.Context, id int) (*biz.PreAuthKey, error) {
	po, err := r.data.db.PreAuthKey.Query().
		Where(preauthkey.ID(id)).
		WithUser().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return entPreAuthKeyToBiz(po), nil
}

func (r *preAuthKeyRepo) ListByUser(ctx context.Context, userID uint) ([]*biz.PreAuthKey, error) {
	pos, err := r.data.db.PreAuthKey.Query().
		Where(preauthkey.HasUserWith(user.ID(int(userID)))).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]*biz.PreAuthKey, 0, len(pos))
	for _, po := range pos {
		keys = append(keys, entPreAuthKeyToBiz(po))
	}
	return keys, nil
}

func (r *preAuthKeyRepo) ListAll(ctx context.Context) ([]*biz.PreAuthKey, error) {
	pos, err := r.data.db.PreAuthKey.Query().
		WithUser().
		All(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]*biz.PreAuthKey, 0, len(pos))
	for _, po := range pos {
		keys = append(keys, entPreAuthKeyToBiz(po))
	}
	return keys, nil
}

func (r *preAuthKeyRepo) Delete(ctx context.Context, id int) error {
	// 先检查是否存在
	exists, err := r.data.db.PreAuthKey.Query().Where(preauthkey.ID(id)).Exist(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return nil // ID不存在时静默返回
	}
	// 删除密钥
	return r.data.db.PreAuthKey.DeleteOneID(id).Exec(ctx)
}

func (r *preAuthKeyRepo) Expire(ctx context.Context, id int) error {
	now := time.Now()
	return r.data.db.PreAuthKey.UpdateOneID(id).
		SetExpiration(now).
		Exec(ctx)
}

func (r *preAuthKeyRepo) IncrementUsage(ctx context.Context, key string) error {
	return r.data.db.PreAuthKey.Update().
		Where(preauthkey.Key(key)).
		AddUsedCount(1).
		Exec(ctx)
}

func entPreAuthKeyToBiz(po *ent.PreAuthKey) *biz.PreAuthKey {
	k := &biz.PreAuthKey{
		ID:        po.ID,
		Key:       po.Key,
		Reusable:  po.Reusable,
		Ephemeral: po.Ephemeral,
		UsedCount: po.UsedCount,
		CreatedAt: po.CreatedAt,
		Tags:      po.ACLTags,
	}
	if po.Expiration != nil {
		k.Expiry = po.Expiration
	}
	if po.Edges.User != nil {
		k.UserID = uint(po.Edges.User.ID)
	}
	return k
}