package controllers

import (
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PVCleanupController struct {
	Client                  client.Client
	DryRun                  bool
	EnableNodeWatchers      bool
	EnablePeriodicCleanup   bool
	NodeSelectorKeys        []string
	PeriodicCleanupInterval time.Duration
}
