# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Kubernetes controller called "experimentor" that manages experiment versions of production workloads. It implements an `ExperimentDeployment` Custom Resource Definition (CRD) that allows users to create smaller "experiment" versions of existing Deployments, StatefulSets, or Argo Rollouts by applying overrides to the source workload's spec.

The controller:
- Watches `ExperimentDeployment` CRs and reconciles them
- Fetches source workloads, applies deep-merged overrides from the CR
- Creates experiment workloads that share the same service as the source
- Automatically cleans up experiment workloads when CRs are deleted
- Currently only supports Deployment workloads (StatefulSet and Rollout support planned)

## Key Architecture

- **API Types**: `api/v1alpha1/experimentdeployment_types.go` - Defines the CRD structure with `SourceRef`, override spec, and status
- **Controller**: `internal/controller/experimentdeployment_controller.go` - Main reconciliation logic with deep merging using `dario.cat/mergo`
- **CRD Manifests**: Auto-generated in `config/crd/bases/` via Kubebuilder
- **RBAC**: Defined in `config/rbac/` for necessary permissions
- **Deployment**: Helm chart in `charts/experiment-controller/` and Kustomize configs in `config/`

The controller uses owner references for automatic cleanup and implements proper finalizers for graceful deletion handling.

## Development Commands

**Build and Test:**
```bash
make build          # Build manager binary
make test           # Run unit tests (requires setup-envtest)
make test-e2e       # Run e2e tests (requires Kind cluster)
make lint           # Run golangci-lint
make lint-fix       # Run golangci-lint with auto-fixes
```

**Code Generation:**
```bash
make manifests      # Generate CRDs and RBAC
make generate       # Generate DeepCopy methods
make fmt            # Format Go code
make vet            # Run go vet
```

**Local Development:**
```bash
make install        # Install CRDs into cluster
make run            # Run controller locally
make uninstall      # Remove CRDs from cluster
```

**Docker and Deployment:**
```bash
make docker-build IMG=<registry>/experimentor:tag
make docker-push IMG=<registry>/experimentor:tag
make deploy IMG=<registry>/experimentor:tag
make undeploy
```

**Dependencies:**
- Go 1.23.0+
- kubectl v1.11.3+
- Access to Kubernetes v1.11.3+ cluster
- Docker 17.03+ for building images
- Kind for e2e testing

**Running Individual Tests:**
```bash
go test ./internal/controller -run TestExperimentDeploymentController
go test ./test/e2e -run TestE2E -v -ginkgo.v
KUBEBUILDER_ASSETS="$(make setup-envtest)" go test ./... -coverprofile cover.out
```

## Testing Strategy

- Unit tests use Kubebuilder's `envtest` framework
- E2e tests require a Kind cluster to be running
- Tests cover CR lifecycle, override merging, service targeting, and cleanup
- Use Ginkgo/Gomega testing framework

## Code Style

- Follow standard Go formatting with `gofmt` and `goimports`
- Enabled linters include: errcheck, govet, staticcheck, revive, ineffassign, unused
- Comments should have proper spacing (enforced by revive)
- API paths and internal paths have relaxed line length limits

## Key Implementation Notes

- Deep merging uses `dario.cat/mergo` library with `mergo.WithOverride` strategy
- Service targeting works by copying source pod labels to experiment pods
- Owner references ensure automatic cleanup via Kubernetes garbage collection
- Uses finalizers for graceful deletion: `experimentdeployments.experimentcontroller.example.com/finalizer`
- Currently supports only Deployment workloads (StatefulSet/Rollout planned)