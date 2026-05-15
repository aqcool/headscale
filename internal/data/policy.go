package data

import (
	"context"

	"github.com/juanfont/headscale-v2/ent"
	"github.com/juanfont/headscale-v2/ent/aclpolicy"
	"github.com/juanfont/headscale-v2/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
)

type policyRepo struct {
	data *Data
	log  *log.Helper
}

// NewPolicyRepo creates a new policy repo.
func NewPolicyRepo(data *Data, logger log.Logger) biz.PolicyRepo {
	return &policyRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

func (r *policyRepo) Get(ctx context.Context) (*biz.Policy, error) {
	po, err := r.data.db.ACLPolicy.Query().
		Order(ent.Desc(aclpolicy.FieldUpdatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return entPolicyToBiz(po), nil
}

func (r *policyRepo) Save(ctx context.Context, data string) (*biz.Policy, error) {
	po, err := r.data.db.ACLPolicy.Create().
		SetPolicy(data).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return entPolicyToBiz(po), nil
}

func entPolicyToBiz(po *ent.ACLPolicy) *biz.Policy {
	return &biz.Policy{
		ID:        po.ID,
		Data:      po.Policy,
		UpdatedAt: po.UpdatedAt,
	}
}