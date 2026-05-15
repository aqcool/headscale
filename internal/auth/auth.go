package auth

import (
	"context"
	"errors"
	"time"

	"github.com/juanfont/headscale-v2/internal/types"
)

var (
	ErrInvalidPreAuthKey = errors.New("invalid pre-auth key")
	ErrExpiredPreAuthKey = errors.New("pre-auth key expired")
	ErrPreAuthKeyUsed    = errors.New("pre-auth key already used")
)

type PreAuthKeyValidator interface {
	ValidateKey(ctx context.Context, key string) (*types.PreAuthKey, error)
	UseKey(ctx context.Context, id uint64) error
}

type AuthProvider struct {
	keyRepo PreAuthKeyValidator
}

func NewAuthProvider(keyRepo PreAuthKeyValidator) *AuthProvider {
	return &AuthProvider{keyRepo: keyRepo}
}

func (a *AuthProvider) ValidatePreAuthKey(ctx context.Context, key string) (*types.PreAuthKey, error) {
	if key == "" {
		return nil, ErrInvalidPreAuthKey
	}

	pak, err := a.keyRepo.ValidateKey(ctx, key)
	if err != nil {
		return nil, err
	}

	if pak == nil {
		return nil, ErrInvalidPreAuthKey
	}

	if !pak.IsValid() {
		if pak.IsExpired() {
			return nil, ErrExpiredPreAuthKey
		}
		if !pak.Reusable && pak.UsedCount() > 0 {
			return nil, ErrPreAuthKeyUsed
		}
		return nil, ErrInvalidPreAuthKey
	}

	return pak, nil
}

func (a *AuthProvider) UsePreAuthKey(ctx context.Context, keyID uint64) error {
	return a.keyRepo.UseKey(ctx, keyID)
}

func (a *AuthProvider) HandleAuth(ctx context.Context, authKey string) (*types.User, []string, error) {
	pak, err := a.ValidatePreAuthKey(ctx, authKey)
	if err != nil {
		return nil, nil, err
	}

	if err := a.UsePreAuthKey(ctx, pak.ID); err != nil {
		return nil, nil, err
	}

	var tags []string
	if len(pak.Tags) > 0 {
		tags = pak.Tags
	}

	return pak.User, tags, nil
}

func (a *AuthProvider) ProcessRegisterRequest(
	ctx context.Context,
	req AuthRegisterRequest,
) (*AuthRegisterResult, error) {
	if req.AuthKey != "" {
		user, tags, err := a.HandleAuth(ctx, req.AuthKey)
		if err != nil {
			return nil, err
		}
		return &AuthRegisterResult{
			UserID:   user.ID,
			Tags:     tags,
			AuthType: "authkey",
		}, nil
	}

	return nil, errors.New("no authentication method provided")
}

type AuthRegisterRequest struct {
	AuthKey    string
	NodeKey    string
	Hostinfo   interface{}
	Expiry     *time.Time
}

type AuthRegisterResult struct {
	UserID   types.UserID
	Tags     []string
	AuthType string
}
