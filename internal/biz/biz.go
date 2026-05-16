package biz

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewUserUsecase,
	NewNodeUsecase,
	NewAPIKeyUsecase,
	NewPreAuthKeyUsecase,
	NewPolicyUsecase,
	NewHeadscaleUsecase,
)