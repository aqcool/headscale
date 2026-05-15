package biz

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// User is a User model.
type User struct {
	ID          int
	Name        string
	DisplayName string
	Email       string
	ProfileURL  string
	CreatedAt   time.Time
}

// UserRepo is a User repo.
type UserRepo interface {
	Save(context.Context, *User) (*User, error)
	Update(context.Context, *User) (*User, error)
	FindByID(context.Context, int) (*User, error)
	FindByName(context.Context, string) (*User, error)
	ListAll(context.Context) ([]*User, error)
	Delete(context.Context, int) error
}

// UserUsecase is a User usecase.
type UserUsecase struct {
	repo UserRepo
	log  *log.Helper
}

// NewUserUsecase new a User usecase.
func NewUserUsecase(repo UserRepo, logger log.Logger) *UserUsecase {
	return &UserUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

// CreateUser creates a User, and returns the new User.
func (uc *UserUsecase) CreateUser(ctx context.Context, user *User) (*User, error) {
	uc.log.Info("CreateUser: ", user.Name)
	return uc.repo.Save(ctx, user)
}

// GetUser gets a User by ID.
func (uc *UserUsecase) GetUser(ctx context.Context, id int) (*User, error) {
	return uc.repo.FindByID(ctx, id)
}

// GetUserByName gets a User by name.
func (uc *UserUsecase) GetUserByName(ctx context.Context, name string) (*User, error) {
	return uc.repo.FindByName(ctx, name)
}

// ListUsers lists all Users.
func (uc *UserUsecase) ListUsers(ctx context.Context) ([]*User, error) {
	return uc.repo.ListAll(ctx)
}

// UpdateUser updates a User.
func (uc *UserUsecase) UpdateUser(ctx context.Context, user *User) (*User, error) {
	return uc.repo.Update(ctx, user)
}

// DeleteUser deletes a User.
func (uc *UserUsecase) DeleteUser(ctx context.Context, id int) error {
	return uc.repo.Delete(ctx, id)
}
