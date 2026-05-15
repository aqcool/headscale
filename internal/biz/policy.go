package biz

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// Policy is a policy model.
type Policy struct {
	ID        int
	Data      string
	UpdatedAt time.Time
}

// PolicyRepo is a Policy repo.
type PolicyRepo interface {
	Get(ctx context.Context) (*Policy, error)
	Save(ctx context.Context, data string) (*Policy, error)
}

// PolicyUsecase is a Policy usecase.
type PolicyUsecase struct {
	repo   PolicyRepo
	polMan *PolicyManager
	log    *log.Helper
}

// PolicyManager manages compiled policies.
type PolicyManager struct {
	policy *Policy
	filter []FilterRule
	ssh    *SSHPolicy
}

// FilterRule represents a filter rule.
type FilterRule struct {
	SrcIPs   []string
	DstPorts []DstPortRange
	Action   string
}

// DstPortRange represents destination port range.
type DstPortRange struct {
	IP    string
	Port  PortRange
}

// PortRange represents port range.
type PortRange struct {
	Start int
	End   int
}

// SSHPolicy represents SSH policy.
type SSHPolicy struct {
	Rules []SSHRule
}

// SSHRule represents SSH rule.
type SSHRule struct {
	SrcIPs   []string
	DstUsers []string
	Action   string
}

// NewPolicyUsecase creates a new Policy usecase.
func NewPolicyUsecase(repo PolicyRepo, logger log.Logger) *PolicyUsecase {
	return &PolicyUsecase{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

// GetPolicy gets the current policy.
func (uc *PolicyUsecase) GetPolicy(ctx context.Context) (*Policy, error) {
	return uc.repo.Get(ctx)
}

// SetPolicy sets a new policy.
func (uc *PolicyUsecase) SetPolicy(ctx context.Context, data string) (*Policy, error) {
	pol, err := uc.repo.Save(ctx, data)
	if err != nil {
		return nil, err
	}

	// Rebuild policy manager
	uc.polMan = &PolicyManager{
		policy: pol,
	}

	return pol, nil
}

// GetFilterRules returns compiled filter rules.
func (uc *PolicyUsecase) GetFilterRules() []FilterRule {
	if uc.polMan == nil {
		return nil
	}
	return uc.polMan.filter
}

// GetSSHPolicy returns compiled SSH policy.
func (uc *PolicyUsecase) GetSSHPolicy() *SSHPolicy {
	if uc.polMan == nil {
		return nil
	}
	return uc.polMan.ssh
}