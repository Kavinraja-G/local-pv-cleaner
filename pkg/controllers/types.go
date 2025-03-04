package controllers

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PVCleanupController struct {
	Client           client.Client
	DryRun           bool
	NodeSelectorKeys []string
}
