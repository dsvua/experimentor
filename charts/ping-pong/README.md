# Ping-Pong Test Application Helm Chart

This Helm chart deploys a simple ping-pong test application that can be used to test the Experiment Controller functionality.

## Installation

```bash
# Install the chart
helm install ping-pong ./charts/ping-pong

# Install with experiment example
helm install ping-pong ./charts/ping-pong \
  --set pingpong.env[0].value="production-pong"
```

## Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Application image repository | `nginx` |
| `image.tag` | Application image tag | `1.21` |
| `replicaCount` | Number of replicas | `3` |
| `service.type` | Service type | `ClusterIP` |
| `service.port` | Service port | `80` |
| `pingpong.env` | Environment variables | See values.yaml |
| `resources.limits.cpu` | CPU limit | `100m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.requests.cpu` | CPU request | `50m` |
| `resources.requests.memory` | Memory request | `64Mi` |

## What it installs

- Deployment of the ping-pong application
- Service to expose the application
- ServiceAccount
- Two example ExperimentDeployments (demonstrate experiment usage)

## Testing with Experiment Controller

This chart includes two example ExperimentDeployments that create experiment versions of the ping-pong app with different environment variables:

```yaml
apiVersion: experimentcontroller.example.com/v1alpha1
kind: ExperimentDeployment
metadata:
  name: ping-pong-experiment
spec:
  sourceRef:
    kind: Deployment
    name: ping-pong
  replicas: 1
  overrideSpec:
    template:
      metadata:
        labels:
          version: experiment
      spec:
        containers:
        - name: ping-pong
          env:
          - name: APP_VERSION
            value: "v1.1.0-experiment"
          - name: RESPONSE_MESSAGE
            value: "pong-experiment"
```

The experiment deployment will:
1. Create a new deployment based on the source ping-pong deployment
2. Override the environment variables to show experimental values
3. Add a `version: experiment` label to distinguish experiment pods
4. Share the same service as the original deployment for traffic splitting