# local-pv-cleaner
Simple K8s controller to clean-up orphaned local PVs (using nvme's) periodically or after nodes get deleted.

## Problem
We often use projects like [TopoLVM](https://github.com/topolvm/topolvm) and [OpenEBS LocalPV](https://openebs.io/docs/2.12.x/concepts/localpv) to provision PersistentVolumes (PVs) backed by ephemeral local-instance storage, such as NVMe disks on AWS EC2 instances. Even when the retention policy is set to `Retain`, the underlying storage is lost when the instance shuts down. This results in orphaned PVs, leading to errors when Kubernetes attempts to reattach them especially if a new node reuses the same IP address. Most of the CSI drivers won't delete the PVs in these scenarios.

To address this, we need a solution that continuously monitors these scenarios and automatically deletes orphaned PVs.

## Features
- **Automatic orphaned PV cleanup**: Identifies and deletes PVs that are not bound to any existing node.
- **Periodic cleanup**: Runs cleanup tasks at a specified interval.
- **Node watcher**: Reacts to node deletions to clean up associated PVs.
- **Dry-run mode**: Allows testing without performing actual deletions.
- **Configurable node selector label**: Supports custom node selector labels for determining PV affinity. Since, CSI drivers define their own topology label.

## Installation
To deploy the Local PV Cleanup Controller in your Kubernetes cluster using Kustomize:
```sh
kubectl apply -k ./deploy
```

## Configuration
The controller supports the following flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | `false` | Run in dry-run mode without making actual changes. |
| `--node-selector-keys` | `topology.topolvm.io/node` | Comma-separated list of labels used in PV node affinity to determine the node name. |
| `--enable-periodic-cleanup` | `true` | Enable periodic cleanup of orphaned PVs. |
| `--enable-node-watchers` | `true` | Enable watching for node deletions and delete local PVs in real-time. |
| `--periodic-cleanup-interval` | `5m` | Interval for periodic orphaned PV cleanup (e.g., 5m, 10m, 1h). |
| `--leader-election` | `true` | Enable leader election for high availability. |
| `--leader-election-id` | `local-pv-cleanup-controller-lock` | Unique leader election ID. |

## Contributing
Feel free to open [issues](https://github.com/Kavinraja-G/local-pv-cleaner/issues/new) or submit PRs if you have any improvements or bug fixes.

## License
[Apache License 2.0](./LICENSE)

