package service

import (
	"context"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/juanfont/headscale-v2/internal/biz"
)

func (s *HeadscaleService) CreateUser(ctx context.Context, req *v1.CreateUserRequest) (*v1.CreateUserResponse, error) {
	user, err := s.userUC.CreateUser(ctx, &biz.User{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Email:       req.Email,
		ProfileURL:  req.PictureUrl,
	})
	if err != nil {
		return nil, err
	}
	return &v1.CreateUserResponse{User: bizUserToProto(user)}, nil
}

func (s *HeadscaleService) ListUsers(ctx context.Context, req *v1.ListUsersRequest) (*v1.ListUsersResponse, error) {
	users, err := s.userUC.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []*biz.User
	switch {
	case req.GetName() != "":
		for _, u := range users {
			if u.Name == req.GetName() {
				filtered = append(filtered, u)
			}
		}
	case req.GetEmail() != "":
		for _, u := range users {
			if u.Email == req.GetEmail() {
				filtered = append(filtered, u)
			}
		}
	case req.GetId() != 0:
		for _, u := range users {
			if uint64(u.ID) == req.GetId() {
				filtered = []*biz.User{u}
				break
			}
		}
	default:
		filtered = users
	}

	protoUsers := make([]*v1.User, 0, len(filtered))
	for _, u := range filtered {
		protoUsers = append(protoUsers, bizUserToProto(u))
	}

	return &v1.ListUsersResponse{Users: protoUsers}, nil
}

func (s *HeadscaleService) DeleteUser(ctx context.Context, req *v1.DeleteUserRequest) (*v1.DeleteUserResponse, error) {
	err := s.userUC.DeleteUser(ctx, int(req.Id))
	if err != nil {
		return nil, err
	}
	return &v1.DeleteUserResponse{}, nil
}

func (s *HeadscaleService) RenameUser(ctx context.Context, req *v1.RenameUserRequest) (*v1.RenameUserResponse, error) {
	user, err := s.userUC.GetUser(ctx, int(req.OldId))
	if err != nil {
		return nil, err
	}
	user.Name = req.NewName
	user, err = s.userUC.UpdateUser(ctx, user)
	if err != nil {
		return nil, err
	}
	return &v1.RenameUserResponse{User: bizUserToProto(user)}, nil
}
