# Experimentor

A Kubernetes controller for creating experiment versions of production workloads. Experimentor allows you to safely test changes by creating modified copies of your existing Deployments, StatefulSets, or Argo Rollouts that share the same service for traffic distribution.

## Description

Experimentor implements an `ExperimentDeployment` Custom Resource Definition (CRD) that enables you to create smaller "experiment" versions of existing workloads by applying selective overrides to the source workload's spec. This is particularly useful for:

- **A/B Testing**: Run different versions of your application with traffic splitting
- **Feature Testing**: Test new features with a subset of users
- **Performance Testing**: Compare resource configurations or different images
- **Configuration Testing**: Test different environment variables or settings

The controller automatically handles:
- Deep merging of override specifications with source workloads
- Service discovery (experiment pods share the same service as source workloads)
- Automatic cleanup when experiments are deleted
- Owner references for proper garbage collection

## How Experiment Creation Works

1. **Source Reference**: You specify a source workload (Deployment, StatefulSet, or Rollout)
2. **Deep Merge**: The controller fetches the source workload and applies your override spec using deep merging
3. **Experiment Creation**: A new workload is created with the merged specification
4. **Service Sharing**: Experiment pods inherit labels from source pods, so they're included in the same service
5. **Traffic Distribution**: Traffic is automatically distributed between source and experiment pods

### Deep Merge Behavior

The controller uses intelligent deep merging:
- **Scalars and objects** are replaced when specified in overrides
- **Container arrays** are merged by name - you only need to specify the fields you want to change
- **Other fields** are preserved from the source workload

Example: To change just the image tag, you only need:
```yaml
overrideSpec:
  template:
    spec:
      containers:
      - name: my-app
        image: my-app:v2.0.0  # Only specify what you want to change
```

All other container properties (ports, env vars, resources, probes, etc.) are automatically preserved.

## Getting Started

### Prerequisites
- go version v1.23.0+
- docker version 17.03+
- kubectl version v1.11.3+
- helm version v3.0+
- Access to a Kubernetes v1.11.3+ cluster

### Installation

#### Using Helm (Recommended)

1. **Install the controller:**
```bash
helm install experiment-controller ./charts/experiment-controller/ \
  --namespace experimentor-system \
  --create-namespace
```

2. **Deploy test applications with experiments:**
```bash
# Deployment-based test app (includes ExperimentDeployment examples)
helm install ping-pong ./charts/ping-pong/ \
  --namespace default

# StatefulSet-based test app (includes ExperimentDeployment examples)
helm install ping-pong-statefulset ./charts/ping-pong-statefulset/ \
  --namespace default

# Argo Rollout-based test app (requires Argo Rollouts controller)
helm install ping-pong-rollout ./charts/ping-pong-rollout/ \
  --namespace default
```

**Note:** The test application charts include pre-configured ExperimentDeployment CRs that automatically create experiment versions of the deployed workloads.

## Creating Experiments

### Basic ExperimentDeployment Structure

```yaml
apiVersion: experimentcontroller.example.com/v1alpha1
kind: ExperimentDeployment
metadata:
  name: my-experiment           # Name of your experiment
  namespace: default           # Should be same namespace as source workload
spec:
  sourceRef:                   # REQUIRED: Reference to source workload
    kind: Deployment           # REQUIRED: Deployment, StatefulSet, or Rollout
    name: my-app              # REQUIRED: Name of source workload
    namespace: default        # REQUIRED: Namespace of source workload
  replicas: 1                 # OPTIONAL: Defaults to 1
  overrideSpec:               # REQUIRED: Overrides to apply
    # Any valid Deployment/StatefulSet/Rollout spec fields
```

### Field Reference

#### Required Fields
- `spec.sourceRef.kind`: Type of source workload (`Deployment`, `StatefulSet`, `Rollout`)
- `spec.sourceRef.name`: Name of the source workload
- `spec.sourceRef.namespace`: Source workload namespace
- `spec.overrideSpec`: Override specification (can be empty `{}` but must be present)

#### Optional Fields
- `spec.replicas`: Number of experiment replicas (defaults to 1)

**Best Practice:** Deploy ExperimentDeployment CRs in the same namespace as their source workloads to ensure proper ServiceAccount access and service discovery.

### Example Use Cases

#### 1. Simple Image Change
Test a new image version while preserving all other settings:

```yaml
apiVersion: experimentcontroller.example.com/v1alpha1
kind: ExperimentDeployment
metadata:
  name: my-app-v2-test
spec:
  sourceRef:
    kind: Deployment
    name: my-app
  replicas: 1
  overrideSpec:
    template:
      spec:
        containers:
        - name: my-app
          image: my-app:v2.0.0
```

#### 2. Environment Variable Changes
Test different configuration settings:

```yaml
apiVersion: experimentcontroller.example.com/v1alpha1
kind: ExperimentDeployment
metadata:
  name: my-app-feature-test
spec:
  sourceRef:
    kind: Deployment
    name: my-app
  replicas: 2
  overrideSpec:
    template:
      metadata:
        labels:
          version: experimental
      spec:
        containers:
        - name: my-app
          env:
          - name: FEATURE_FLAG_X
            value: "enabled"
          - name: LOG_LEVEL
            value: "debug"
```

#### 3. Resource Configuration Testing
Test different resource allocations:

```yaml
apiVersion: experimentcontroller.example.com/v1alpha1
kind: ExperimentDeployment
metadata:
  name: my-app-optimized-resources
spec:
  sourceRef:
    kind: Deployment
    name: my-app
  overrideSpec:
    template:
      spec:
        containers:
        - name: my-app
          resources:
            requests:
              cpu: 200m
              memory: 256Mi
            limits:
              cpu: 500m
              memory: 512Mi
```

#### 4. StatefulSet Experiments
Test changes to StatefulSet workloads:

```yaml
apiVersion: experimentcontroller.example.com/v1alpha1
kind: ExperimentDeployment
metadata:
  name: my-statefulset-experiment
spec:
  sourceRef:
    kind: StatefulSet
    name: my-database
  replicas: 1
  overrideSpec:
    template:
      spec:
        containers:
        - name: my-database
          image: my-database:v2.0.0
          env:
          - name: DB_CACHE_SIZE
            value: "256MB"
```

#### 5. Argo Rollout Experiments
Test changes to Argo Rollout workloads:

```yaml
apiVersion: experimentcontroller.example.com/v1alpha1
kind: ExperimentDeployment
metadata:
  name: my-rollout-experiment
  namespace: default
spec:
  sourceRef:
    kind: Rollout
    name: my-app-rollout
    namespace: default
  replicas: 1
  overrideSpec:
    template:
      spec:
        containers:
        - name: my-app
          image: my-app:canary-build
          env:
          - name: CANARY_FEATURE
            value: "enabled"
```

## Monitoring Experiments

### Check Experiment Status
```bash
kubectl get experimentdeployment
kubectl describe experimentdeployment my-experiment
```

### View Experiment Pods
```bash
kubectl get pods -l experiment-controller.example.com/role=experiment
```

### Check Service Endpoints
Verify both source and experiment pods are receiving traffic:
```bash
kubectl get endpoints my-app-service
```

### Examine Experiment Workload
```bash
kubectl get deployment my-experiment
kubectl describe deployment my-experiment
```

## Troubleshooting

### Common Issues

1. **Experiment Not Creating Pods**
   - Check if the source workload exists and is in the correct namespace
   - Verify the override spec is valid YAML
   - Check controller logs: `kubectl logs -n experimentor-system deployment/experiment-controller`

2. **Experiment Pods Not Receiving Traffic**
   - Ensure experiment pods have the same labels as source pods for service selection
   - Check service selector: `kubectl describe service my-app-service`
   - Verify endpoints: `kubectl get endpoints my-app-service`

3. **Merge Issues**
   - The controller preserves all source workload properties except those explicitly overridden
   - For containers, specify the container name to target specific containers
   - Check the generated experiment workload: `kubectl get deployment my-experiment -o yaml`

### Cleanup

```bash
# Delete specific experiment
kubectl delete experimentdeployment my-experiment

# Delete all experiments
kubectl delete experimentdeployment --all

# Uninstall controller
helm uninstall experiment-controller -n experimentor-system
```

## Limitations

- Experiment workloads are created in the same namespace as the source workload
- Service sharing works through label inheritance - custom service selectors may need adjustment
- Supports Deployment, StatefulSet, and Argo Rollout workloads (Rollouts require Argo Rollouts controller)
- Cross-namespace experiments are supported but should be used carefully due to ServiceAccount constraints

## Development

```bash
# Build locally
make build

# Run tests
make test

# Run end-to-end tests (requires Kind cluster)
make test-e2e

# Code generation
make manifests generate

# Linting
make lint
```

For detailed development instructions, see [CLAUDE.md](./CLAUDE.md).

## License

This project is dual-licensed under your choice of either:

- **Apache License 2.0** ([LICENSE-APACHE](LICENSE-APACHE))
- **MIT License** ([LICENSE-MIT](LICENSE-MIT))

You may use this software under the terms of either license.

### Apache License 2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

### MIT License

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.