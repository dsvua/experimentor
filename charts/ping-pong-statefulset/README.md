# ping-pong-statefulset

A Helm chart for testing StatefulSet deployments with the experimentor controller.

## Overview

This chart deploys a simple ping-pong application using a StatefulSet instead of a standard Deployment. It includes:

- A StatefulSet with persistent storage
- Regular service and headless service
- ServiceAccount for pod identity
- Example ExperimentDeployment CR for testing

## Features

- **Persistent Storage**: Each pod gets its own persistent volume
- **Headless Service**: For StatefulSet pod discovery
- **Ordered Deployment**: Pods are created and updated in order
- **Stable Network Identity**: Each pod has a predictable hostname

## Installation

```bash
helm install my-ping-pong-statefulset ./ping-pong-statefulset
```

## Configuration

Key configuration options:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `3` |
| `image.repository` | Container image repository | `httpd` |
| `image.tag` | Container image tag | `2.4` |
| `persistence.enabled` | Enable persistent storage | `true` |
| `persistence.size` | Size of persistent volume | `1Gi` |
| `statefulset.serviceName` | Headless service name | `ping-pong-statefulset-headless` |
| `pingpong.env` | Environment variables | See values.yaml |

## Storage

The StatefulSet uses volumeClaimTemplates to create persistent volumes for each pod. Data is mounted at `/var/www/html/data`.

## Testing with ExperimentDeployment

The chart includes an example ExperimentDeployment CR that creates an experiment version of the StatefulSet with:
- Reduced replica count (1 instead of 3)
- Modified environment variables
- Reduced resource limits

The ExperimentDeployment is automatically deployed with the chart and will create an experiment StatefulSet in the same namespace.