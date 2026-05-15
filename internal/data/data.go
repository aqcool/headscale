package data

import (
	"context"

	"github.com/juanfont/headscale-v2/ent"
	"github.com/juanfont/headscale-v2/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	_ "github.com/mattn/go-sqlite3"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewUserRepo, NewNodeRepo, NewAPIKeyRepo, NewPreAuthKeyRepo, NewPolicyRepo)

// Data .
type Data struct {
	db *ent.Client
}

// NewData .
func NewData(c *conf.Data, logger log.Logger) (*Data, func(), error) {
	log.NewHelper(logger).Info("connecting to database:", c.Database.Source)

	client, err := ent.Open(
		c.Database.Driver,
		c.Database.Source,
	)
	if err != nil {
		log.NewHelper(logger).Errorf("failed opening connection to sqlite: %v", err)
		return nil, nil, err
	}

	// Run the auto migration tool.
	if err := client.Schema.Create(context.Background()); err != nil {
		log.NewHelper(logger).Errorf("failed creating schema resources: %v", err)
		return nil, nil, err
	}

	cleanup := func() {
		log.NewHelper(logger).Info("closing the data resources")
		if err := client.Close(); err != nil {
			log.NewHelper(logger).Errorf("failed closing database: %v", err)
		}
	}
	return &Data{db: client}, cleanup, nil
}
