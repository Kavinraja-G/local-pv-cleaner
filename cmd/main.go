/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"time"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	localPVCleaner "github.com/Kavinraja-G/local-pv-cleaner/pkg/controllers"
)

// Config struct for controller settings
type Config struct {
	LeaderElection          bool
	LeaderElectionID        string
	DryRun                  bool
	EnablePeriodicCleanup   bool
	EnableNodeWatchers      bool
	NodeSelectorKeys        []string
	PeriodicCleanupInterval time.Duration
}

// parseFlags sets and parses the controller arguments
func parseFlags() *Config {
	cfg := &Config{}

	pflag.BoolVar(&cfg.LeaderElection, "leader-election", true, "Enable leader election for high availability")
	pflag.StringVar(&cfg.LeaderElectionID, "leader-election-id", "local-pv-cleanup-controller-lock", "Unique leader election ID")
	pflag.BoolVar(&cfg.DryRun, "dry-run", false, "Run in dry-run mode without making actual changes")
	pflag.StringSliceVar(&cfg.NodeSelectorKeys, "node-selector-keys", []string{"topology.topolvm.io/node"}, "Comma-separated list of labels used in PV node affinity to determine the node name")
	pflag.BoolVar(&cfg.EnablePeriodicCleanup, "enable-periodic-cleanup", true, "Enable periodic cleanup of orphaned PVs")
	pflag.BoolVar(&cfg.EnableNodeWatchers, "enable-node-watchers", true, "Enable watching for node deletions and delete local PVs in real-time")
	pflag.DurationVar(&cfg.PeriodicCleanupInterval, "periodic-cleanup-interval", 5*time.Minute, "Interval for periodic orphaned PV cleanup (e.g., 5m, 10m, 1h)")
	pflag.Parse()

	return cfg
}

func main() {
	ctrl.SetLogger(klog.NewKlogr())
	logger := klog.FromContext(context.Background())

	cfg := parseFlags()

	// Print all flag values
	pflag.VisitAll(func(flag *pflag.Flag) {
		klog.Infof("Flag --%s=%s", flag.Name, flag.Value)
	})

	mgrOptions := manager.Options{
		LeaderElection:   cfg.LeaderElection,
		LeaderElectionID: cfg.LeaderElectionID,
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOptions)
	if err != nil {
		logger.Error(err, "Failed to create manager")
	}

	controller := &localPVCleaner.PVCleanupController{
		Client:                  mgr.GetClient(),
		DryRun:                  cfg.DryRun,
		PeriodicCleanupInterval: cfg.PeriodicCleanupInterval,
		EnableNodeWatchers:      cfg.EnableNodeWatchers,
		EnablePeriodicCleanup:   cfg.EnablePeriodicCleanup,
		NodeSelectorKeys:        cfg.NodeSelectorKeys,
	}

	// Periodic clean-up of local PVs and enabled by default when nodeWatchers are disabled
	if cfg.EnablePeriodicCleanup || !cfg.EnableNodeWatchers {
		go func() {
			err := controller.PeriodicPVCleanup(context.Background())
			if err != nil {
				logger.Error(err, "PeriodicPVCleanup failed")
			}
		}()
	}

	// Watch for node delete events and clean-up local PVs in real-time
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		WithEventFilter(predicate.Funcs{
			GenericFunc: func(genericEvent event.GenericEvent) bool {
				return false
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				return false
			},
			CreateFunc: func(createEvent event.CreateEvent) bool {
				return false
			},
		}).
		Complete(controller); err != nil {
		logger.Error(err, "Failed to create controller")
		return
	}

	logger.Info("Starting local-pv-cleaner manager...")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Error(err, "Manager exited non-zero")
	}
}
