# Experiment Controller Helm Chart

This Helm chart deploys the Experiment Controller, a Kubernetes operator that manages experiment versions of production workloads.

## Installation

```bash
# Install the chart
helm install experiment-controller ./charts/experiment-controller

# Install in a custom namespace
helm install experiment-controller ./charts/experiment-controller \
  --namespace experimentor-system \
  --create-namespace

# Install with custom image
helm install experiment-controller ./charts/experiment-controller \
  --set image.repository=myregistry/experimentor \
  --set image.tag=v1.0.0
```

## Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Controller image repository | `controller` |
| `image.tag` | Controller image tag | `latest` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `replicaCount` | Number of controller replicas | `1` |
| `namespace.create` | Create namespace | `true` |
| `namespace.name` | Namespace name | `""` (uses Release.Namespace) |
| `serviceAccount.create` | Create service account | `true` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.requests.cpu` | CPU request | `10m` |
| `resources.requests.memory` | Memory request | `64Mi` |

## What it installs

- CustomResourceDefinition for ExperimentDeployment
- RBAC (ClusterRole and ClusterRoleBinding)
- Deployment of the controller
- Service for health checks
- ServiceAccount

## Usage

After installation, you can create ExperimentDeployment resources to experiment with your workloads:

```yaml
apiVersion: experimentcontroller.example.com/v1alpha1
kind: ExperimentDeployment
metadata:
  name: my-experiment
  namespace: default  # Same namespace as source workload
spec:
  sourceRef:
    kind: Deployment
    name: my-app
    namespace: default
  replicas: 1
  overrideSpec:
    template:
      spec:
        containers:
        - name: app
          env:
          - name: FEATURE_FLAG
            value: "enabled"
```

**Best Practice:** Deploy ExperimentDeployment CRs in the same namespace as their source workloads. See the ping-pong chart examples for reference implementations.