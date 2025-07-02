# ping-pong-rollout

A Helm chart for testing Argo Rollout deployments with the experimentor controller.

## Overview

This chart deploys a simple ping-pong application using an Argo Rollout instead of a standard Deployment. It includes:

- An Argo Rollout with canary deployment strategy
- Service for network access
- ServiceAccount for pod identity
- Example ExperimentDeployment CR for testing

## Prerequisites

- Kubernetes 1.16+
- Argo Rollouts controller installed in the cluster
- Helm 3.0+

## Installing Argo Rollouts

If you don't have Argo Rollouts installed, you can install it with:

```bash
kubectl create namespace argo-rollouts
kubectl apply -n argo-rollouts -f https://github.com/argoproj/argo-rollouts/releases/latest/download/install.yaml
```

## Installation

```bash
helm install my-ping-pong-rollout ./ping-pong-rollout
```

## Configuration

Key configuration options:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `3` |
| `image.repository` | Container image repository | `httpd` |
| `image.tag` | Container image tag | `2.4` |
| `rollout.strategy.canary` | Canary deployment strategy | See values.yaml |
| `pingpong.env` | Environment variables | See values.yaml |

## Testing with ExperimentDeployment

The chart includes an example ExperimentDeployment CR that creates an experiment version of the rollout with:
- Reduced replica count (1 instead of 3)
- Modified environment variables
- Reduced resource limits

The ExperimentDeployment is automatically deployed with the chart and will create an experiment Rollout in the same namespace.

**Note:** This chart requires the Argo Rollouts controller to be installed in your cluster. The ExperimentDeployment will create a functional experiment Rollout with simplified deployment strategy.