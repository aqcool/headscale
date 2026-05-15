package server

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// NodesGauge tracks the number of registered nodes
	NodesGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "headscale_nodes_total",
		Help: "Total number of registered nodes",
	})

	// UsersGauge tracks the number of users
	UsersGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "headscale_users_total",
		Help: "Total number of users",
	})

	// ConnectedNodesGauge tracks currently connected nodes
	ConnectedNodesGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "headscale_nodes_connected",
		Help: "Number of currently connected nodes",
	})

	// MapResponseCounter counts MapResponse sent
	MapResponseCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "headscale_map_responses_total",
		Help: "Total number of MapResponse sent",
	})

	// RegisterCounter counts node registrations
	RegisterCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "headscale_registrations_total",
		Help: "Total number of node registrations",
	})
)
