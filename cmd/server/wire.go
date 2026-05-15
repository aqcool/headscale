//go:build wireinject
// +build wireinject

package main

import (
	"github.com/juanfont/headscale-v2/internal/biz"
	"github.com/juanfont/headscale-v2/internal/conf"
	"github.com/juanfont/headscale-v2/internal/data"
	"github.com/juanfont/headscale-v2/internal/server"
	"github.com/juanfont/headscale-v2/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

func wireApp(*conf.Server, *conf.Data, *conf.Headscale, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(server.ProviderSet, data.ProviderSet, biz.ProviderSet, service.ProviderSet, newApp))
}