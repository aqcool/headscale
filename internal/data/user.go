package data

import (
	"context"

	"github.com/juanfont/headscale-v2/ent"
	"github.com/juanfont/headscale-v2/ent/user"
	"github.com/juanfont/headscale-v2/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
)

type userRepo struct {
	data *Data
	log  *log.Helper
}

// NewUserRepo new a user repo.
func NewUserRepo(data *Data, logger log.Logger) biz.UserRepo {
	return &userRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

func (r *userRepo) Save(ctx context.Context, u *biz.User) (*biz.User, error) {
	po, err := r.data.db.User.
		Create().
		SetName(u.Name).
		SetDisplayName(u.DisplayName).
		SetEmail(u.Email).
		SetProfilePicURL(u.ProfileURL).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return entUserToBiz(po), nil
}

func (r *userRepo) Update(ctx context.Context, u *biz.User) (*biz.User, error) {
	po, err := r.data.db.User.
		UpdateOneID(u.ID).
		SetName(u.Name).
		SetDisplayName(u.DisplayName).
		SetEmail(u.Email).
		SetProfilePicURL(u.ProfileURL).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return entUserToBiz(po), nil
}

func (r *userRepo) FindByID(ctx context.Context, id int) (*biz.User, error) {
	po, err := r.data.db.User.Query().Where(user.ID(id)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return entUserToBiz(po), nil
}

func (r *userRepo) FindByName(ctx context.Context, name string) (*biz.User, error) {
	po, err := r.data.db.User.Query().Where(user.Name(name)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return entUserToBiz(po), nil
}

func (r *userRepo) ListAll(ctx context.Context) ([]*biz.User, error) {
	pos, err := r.data.db.User.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]*biz.User, 0, len(pos))
	for _, po := range pos {
		users = append(users, entUserToBiz(po))
	}
	return users, nil
}

func (r *userRepo) Delete(ctx context.Context, id int) error {
	return r.data.db.User.DeleteOneID(id).Exec(ctx)
}

func entUserToBiz(po *ent.User) *biz.User {
	return &biz.User{
		ID:          po.ID,
		Name:        po.Name,
		DisplayName: po.DisplayName,
		Email:       po.Email,
		ProfileURL:  po.ProfilePicURL,
		CreatedAt:   po.CreatedAt,
	}
}
