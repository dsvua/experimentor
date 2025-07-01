# Kustomize Deployment for Experiment Controller

This directory contains Kustomize configurations for deploying the Experiment Controller as an alternative to Helm charts.

## Directory Structure

```
kustomize/
├── base/                           # Base configuration
│   ├── kustomization.yaml         # Base kustomization
│   ├── namespace.yaml             # Namespace definition
│   ├── serviceaccount.yaml        # ServiceAccount
│   ├── clusterrole.yaml          # ClusterRole (cluster-wide permissions)
│   ├── clusterrolebinding.yaml   # ClusterRoleBinding
│   ├── service.yaml              # Service for health checks
│   └── deployment.yaml           # Controller deployment
└── overlays/                     # Environment-specific configurations
    ├── development/              # Development overlay
    │   ├── kustomization.yaml   # Dev-specific configuration
    │   └── deployment-patch.yaml # Resource and config patches
    ├── production/               # Production overlay
    │   ├── kustomization.yaml   # Prod-specific configuration
    │   ├── deployment-patch.yaml # High-availability configuration
    │   └── hpa.yaml             # Horizontal Pod Autoscaler
    └── namespace-scoped/         # Namespace-scoped deployment
        ├── kustomization.yaml   # Namespace-scoped configuration
        ├── role.yaml           # Role (namespace permissions)
        ├── rolebinding.yaml    # RoleBinding
        ├── remove-clusterrole.yaml # Remove cluster-wide permissions
        └── deployment-patch.yaml   # Watch namespaces configuration
```

## Quick Start

### Prerequisites

- kubectl v1.14+ (with kustomize support)
- Access to a Kubernetes cluster
- CRDs installed (see below)

### Install CRDs First

Before deploying the controller, install the Custom Resource Definitions:

```bash
kubectl apply -f config/crd/bases/experimentcontroller.example.com_experimentdeployments.yaml
```

### Deploy with Base Configuration

```bash
# Deploy with default settings
kubectl apply -k kustomize/base/

# Verify deployment
kubectl get pods -n experiment-system
kubectl get experimentdeployments --all-namespaces
```

## Environment-Specific Deployments

### Development Environment

Optimized for local development with minimal resources:

```bash
kubectl apply -k kustomize/overlays/development/
```

Features:
- Single replica
- Reduced resource limits (200m CPU, 128Mi memory)
- Leader election disabled
- Debug logging
- Latest image tag

### Production Environment

Production-ready configuration with high availability:

```bash
# Update the image registry in overlays/production/kustomization.yaml first
kubectl apply -k kustomize/overlays/production/
```

Features:
- 3 replicas with pod anti-affinity
- Higher resource limits (1000m CPU, 512Mi memory)
- Horizontal Pod Autoscaler (3-10 replicas)
- Secure metrics endpoint
- Versioned image from production registry

### Namespace-Scoped Deployment

Deploy controller with namespace-scoped permissions instead of cluster-wide:

```bash
kubectl apply -k kustomize/overlays/namespace-scoped/
```

Features:
- Role/RoleBinding instead of ClusterRole/ClusterRoleBinding
- Controller watches only `experiment-system` namespace
- Reduced security footprint

## Customization

### Changing the Image

Edit the image configuration in the appropriate kustomization.yaml:

```yaml
images:
  - name: experimentor
    newName: your-registry.com/experimentor
    newTag: v1.0.0
```

### Modifying Resources

Create a patch file or edit existing ones:

```yaml
# deployment-patch.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: experiment-controller
spec:
  template:
    spec:
      containers:
        - name: experiment-controller-manager
          resources:
            limits:
              cpu: 2000m
              memory: 1Gi
            requests:
              cpu: 500m
              memory: 512Mi
```

### Adding Custom Configuration

Use configMapGenerator to add environment-specific configuration:

```yaml
configMapGenerator:
  - name: controller-config
    behavior: merge
    literals:
      - WATCH_NAMESPACES=namespace1,namespace2
      - LOG_LEVEL=debug
      - CUSTOM_CONFIG=value
```

### Watch Specific Namespaces

To make the controller watch specific namespaces:

```yaml
# In your overlay's deployment-patch.yaml
spec:
  template:
    spec:
      containers:
        - name: experiment-controller-manager
          args:
            - --leader-elect
            - --health-probe-bind-address=:8081
            - --watch-namespaces=namespace1,namespace2,namespace3
```

## Verification

### Check Controller Status

```bash
# Check pods
kubectl get pods -n experiment-system

# Check controller logs
kubectl logs -n experiment-system deployment/experiment-controller

# Check CRDs
kubectl get crd experimentdeployments.experimentcontroller.example.com

# Test with a sample ExperimentDeployment
kubectl apply -f config/samples/experimentcontroller.example.com_v1alpha1_experimentdeployment_rollout.yaml
```

### Monitor Health

```bash
# Check service endpoints
kubectl get endpoints -n experiment-system

# Test health endpoint (if service is exposed)
kubectl port-forward -n experiment-system svc/experiment-controller 8081:8081
curl http://localhost:8081/healthz
curl http://localhost:8081/readyz
```

## Cleanup

```bash
# Remove specific overlay
kubectl delete -k kustomize/overlays/production/

# Remove base installation
kubectl delete -k kustomize/base/

# Remove CRDs (this will delete all ExperimentDeployment resources)
kubectl delete -f config/crd/bases/experimentcontroller.example.com_experimentdeployments.yaml
```

## Comparison with Helm

| Feature | Kustomize | Helm |
|---------|-----------|------|
| **Templating** | Patch-based | Template-based |
| **Values** | ConfigMap/patches | values.yaml |
| **Environments** | Overlays | Multiple value files |
| **Complexity** | Moderate | Lower |
| **Flexibility** | High | High |
| **Learning Curve** | Steeper | Gentler |

Choose Kustomize if you:
- Prefer declarative configuration management
- Want to avoid templating syntax
- Need fine-grained control over patches
- Already use Kustomize in your workflow

Choose Helm if you:
- Want simpler value-based configuration
- Prefer package management features
- Need easier upgrade/rollback capabilities
- Are new to Kubernetes deployments

## Troubleshooting

### Common Issues

1. **CRDs not found**: Install CRDs first using `kubectl apply -f config/crd/bases/`
2. **Permission denied**: Check RBAC configuration in overlays
3. **Image pull errors**: Update image name/tag in kustomization.yaml
4. **Controller not starting**: Check resource limits and node capacity

### Debugging Commands

```bash
# Check kustomize output without applying
kubectl kustomize kustomize/overlays/production/

# Describe resources
kubectl describe deployment -n experiment-system experiment-controller

# Check events
kubectl get events -n experiment-system --sort-by=.metadata.creationTimestamp
```