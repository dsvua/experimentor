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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"dario.cat/mergo"
	rolloutsv1alpha1 "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	experimentcontrollercomv1alpha1 "experimentcontroller.example.com/experiment-deployment/api/v1alpha1"
)

const (
	experimentDeploymentFinalizer = "experimentdeployments.experimentcontroller.example.com/finalizer"
	// Condition Types
	ConditionTypeReady     = "Ready"
	ConditionTypeSynced    = "Synced"
	ReasonReconcileError   = "ReconcileError"
	ReasonReconcileSuccess = "ReconcileSuccess"
	// Label values
	ExperimentRoleValue = "experiment"
)

// ExperimentDeploymentReconciler reconciles a ExperimentDeployment object
type ExperimentDeploymentReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=experimentcontroller.example.com,resources=experimentdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=experimentcontroller.example.com,resources=experimentdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=experimentcontroller.example.com,resources=experimentdeployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=argoproj.io,resources=rollouts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ExperimentDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	experimentCR := &experimentcontrollercomv1alpha1.ExperimentDeployment{}
	if err := r.Get(ctx, req.NamespacedName, experimentCR); err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("ExperimentDeployment resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get ExperimentDeployment")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !experimentCR.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(experimentCR, experimentDeploymentFinalizer) {
			log.Info("Handling deletion of ExperimentDeployment", "name", experimentCR.Name)

			// Delete the experiment Deployment
			experimentDeploymentName := fmt.Sprintf("%s-experiment", experimentCR.Spec.SourceRef.Name) // Assuming source name is stable for this
			err := r.deleteExperimentDeployment(ctx, experimentCR.Namespace, experimentDeploymentName)
			if err != nil {
				log.Error(err, "Failed to delete experiment Deployment during finalization")
				// If deletion fails, return error so it can be retried
				return ctrl.Result{}, err
			}
			log.Info("Successfully deleted experiment Deployment", "name", experimentDeploymentName)

			controllerutil.RemoveFinalizer(experimentCR, experimentDeploymentFinalizer)
			if err := r.Update(ctx, experimentCR); err != nil {
				log.Error(err, "Failed to remove finalizer from ExperimentDeployment")
				return ctrl.Result{}, err
			}
			log.Info("Successfully removed finalizer from ExperimentDeployment")
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(experimentCR, experimentDeploymentFinalizer) {
		log.Info("Adding finalizer to ExperimentDeployment", "name", experimentCR.Name)
		controllerutil.AddFinalizer(experimentCR, experimentDeploymentFinalizer)
		if err := r.Update(ctx, experimentCR); err != nil {
			log.Error(err, "Failed to add finalizer to ExperimentDeployment")
			return ctrl.Result{}, err
		}
		// Requeue because the update will trigger another reconcile
		return ctrl.Result{Requeue: true}, nil
	}

	// Check source kind - ensure it's supported
	switch experimentCR.Spec.SourceRef.Kind {
	case experimentcontrollercomv1alpha1.SourceKindDeployment,
		experimentcontrollercomv1alpha1.SourceKindStatefulSet,
		experimentcontrollercomv1alpha1.SourceKindRollout:
		// Supported kinds, continue
	default:
		err := fmt.Errorf("unsupported source kind '%s', supported kinds are: %s, %s, %s",
			experimentCR.Spec.SourceRef.Kind,
			experimentcontrollercomv1alpha1.SourceKindDeployment,
			experimentcontrollercomv1alpha1.SourceKindStatefulSet,
			experimentcontrollercomv1alpha1.SourceKindRollout)
		log.Error(err, "Invalid SourceRef.Kind")
		r.Recorder.Event(experimentCR, corev1.EventTypeWarning, "UnsupportedSourceKind", err.Error())
		// Update status
		meta.SetStatusCondition(&experimentCR.Status.Conditions, metav1.Condition{
			Type:    ConditionTypeSynced,
			Status:  metav1.ConditionFalse,
			Reason:  "UnsupportedSourceKind",
			Message: err.Error(),
		})
		meta.SetStatusCondition(&experimentCR.Status.Conditions, metav1.Condition{
			Type:    ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  "UnsupportedSourceKind",
			Message: err.Error(),
		})
		// Use the safer status update method
		_, updateErr := r.finalizeStatusUpdate(ctx, experimentCR)
		if updateErr != nil {
			log.Error(updateErr, "Failed to update ExperimentDeployment status for unsupported kind")
		}
		return ctrl.Result{}, nil // No requeue, wait for user to fix CR
	}

	// Reconcile experiment workload based on source kind
	experimentWorkload, err := r.reconcileExperimentWorkload(ctx, experimentCR)
	if err != nil {
		return ctrl.Result{}, err
	}
	if experimentWorkload == nil {
		// Requeue needed, but first update status with error conditions
		result, err := r.finalizeStatusUpdate(ctx, experimentCR)
		if err != nil {
			return result, err
		}
		// Override the result to use 30-second requeue for source not found scenarios
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Update Status
	return r.updateExperimentWorkloadStatus(ctx, experimentCR, experimentWorkload)
}

// reconcileExperimentWorkload handles fetching source workload and creating experiment workload for all supported kinds
func (r *ExperimentDeploymentReconciler) reconcileExperimentWorkload(ctx context.Context, experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment) (client.Object, error) {
	log := logf.FromContext(ctx)

	sourceNamespace := experimentCR.Spec.SourceRef.Namespace
	if sourceNamespace == "" {
		sourceNamespace = experimentCR.Namespace
	}

	switch experimentCR.Spec.SourceRef.Kind {
	case experimentcontrollercomv1alpha1.SourceKindDeployment:
		return r.reconcileDeploymentExperiment(ctx, experimentCR, sourceNamespace)
	case experimentcontrollercomv1alpha1.SourceKindStatefulSet:
		return r.reconcileStatefulSetExperiment(ctx, experimentCR, sourceNamespace)
	case experimentcontrollercomv1alpha1.SourceKindRollout:
		// Check if Rollouts are supported in this cluster
		if !r.isRolloutSupported() {
			err := fmt.Errorf("Argo Rollouts are not installed in this cluster, cannot process Rollout source kind")
			log.Error(err, "Rollouts not supported")
			r.Recorder.Event(experimentCR, corev1.EventTypeWarning, "RolloutsNotSupported", err.Error())
			r.updateStatusConditions(experimentCR, "RolloutsNotSupported", err.Error())
			return nil, err
		}
		return r.reconcileRolloutExperiment(ctx, experimentCR, sourceNamespace)
	default:
		err := fmt.Errorf("unsupported source kind: %s", experimentCR.Spec.SourceRef.Kind)
		log.Error(err, "Invalid source kind")
		return nil, err
	}
}

// reconcileDeploymentExperiment handles Deployment-based experiments
func (r *ExperimentDeploymentReconciler) reconcileDeploymentExperiment(ctx context.Context, experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment, sourceNamespace string) (client.Object, error) {
	log := logf.FromContext(ctx)

	// Fetch source Deployment
	sourceDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: experimentCR.Spec.SourceRef.Name, Namespace: sourceNamespace}, sourceDeployment)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Error(err, "Source Deployment not found", "sourceName", experimentCR.Spec.SourceRef.Name, "sourceNamespace", sourceNamespace)
			r.Recorder.Eventf(experimentCR, corev1.EventTypeWarning, "SourceNotFound", "Source Deployment %s/%s not found", sourceNamespace, experimentCR.Spec.SourceRef.Name)
			r.updateStatusConditions(experimentCR, "SourceNotFound", fmt.Sprintf("Source Deployment %s/%s not found", sourceNamespace, experimentCR.Spec.SourceRef.Name))
			return nil, nil // Return nil to indicate requeue needed
		}
		log.Error(err, "Failed to get source Deployment")
		return nil, err
	}

	// Construct experiment Deployment
	desiredExperimentDeployment, err := r.constructExperimentDeployment(experimentCR, sourceDeployment)
	if err != nil {
		log.Error(err, "Failed to construct desired experiment Deployment")
		r.Recorder.Eventf(experimentCR, corev1.EventTypeWarning, "ConstructionFailed", "Failed to construct experiment Deployment: %s", err.Error())
		r.updateStatusConditions(experimentCR, "ConstructionFailed", fmt.Sprintf("Failed to construct experiment Deployment: %s", err.Error()))
		return nil, err
	}

	// Create or Update experiment Deployment
	return r.createOrUpdateDeployment(ctx, experimentCR, desiredExperimentDeployment)
}

// reconcileStatefulSetExperiment handles StatefulSet-based experiments
func (r *ExperimentDeploymentReconciler) reconcileStatefulSetExperiment(ctx context.Context, experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment, sourceNamespace string) (client.Object, error) {
	log := logf.FromContext(ctx)

	// Fetch source StatefulSet
	sourceStatefulSet := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: experimentCR.Spec.SourceRef.Name, Namespace: sourceNamespace}, sourceStatefulSet)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Error(err, "Source StatefulSet not found", "sourceName", experimentCR.Spec.SourceRef.Name, "sourceNamespace", sourceNamespace)
			r.Recorder.Eventf(experimentCR, corev1.EventTypeWarning, "SourceNotFound", "Source StatefulSet %s/%s not found", sourceNamespace, experimentCR.Spec.SourceRef.Name)
			r.updateStatusConditions(experimentCR, "SourceNotFound", fmt.Sprintf("Source StatefulSet %s/%s not found", sourceNamespace, experimentCR.Spec.SourceRef.Name))
			return nil, nil // Return nil to indicate requeue needed
		}
		log.Error(err, "Failed to get source StatefulSet")
		return nil, err
	}

	// Construct experiment StatefulSet
	desiredExperimentStatefulSet, err := r.constructExperimentStatefulSet(experimentCR, sourceStatefulSet)
	if err != nil {
		log.Error(err, "Failed to construct desired experiment StatefulSet")
		r.Recorder.Eventf(experimentCR, corev1.EventTypeWarning, "ConstructionFailed", "Failed to construct experiment StatefulSet: %s", err.Error())
		r.updateStatusConditions(experimentCR, "ConstructionFailed", fmt.Sprintf("Failed to construct experiment StatefulSet: %s", err.Error()))
		return nil, err
	}

	// Create or Update experiment StatefulSet
	return r.createOrUpdateStatefulSet(ctx, experimentCR, desiredExperimentStatefulSet)
}

// reconcileRolloutExperiment handles Argo Rollout-based experiments
func (r *ExperimentDeploymentReconciler) reconcileRolloutExperiment(ctx context.Context, experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment, sourceNamespace string) (client.Object, error) {
	log := logf.FromContext(ctx)

	// Fetch source Rollout
	sourceRollout := &rolloutsv1alpha1.Rollout{}
	err := r.Get(ctx, types.NamespacedName{Name: experimentCR.Spec.SourceRef.Name, Namespace: sourceNamespace}, sourceRollout)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Error(err, "Source Rollout not found", "sourceName", experimentCR.Spec.SourceRef.Name, "sourceNamespace", sourceNamespace)
			r.Recorder.Eventf(experimentCR, corev1.EventTypeWarning, "SourceNotFound", "Source Rollout %s/%s not found", sourceNamespace, experimentCR.Spec.SourceRef.Name)
			r.updateStatusConditions(experimentCR, "SourceNotFound", fmt.Sprintf("Source Rollout %s/%s not found", sourceNamespace, experimentCR.Spec.SourceRef.Name))
			return nil, nil // Return nil to indicate requeue needed
		}
		log.Error(err, "Failed to get source Rollout")
		return nil, err
	}

	// Construct experiment Rollout
	desiredExperimentRollout, err := r.constructExperimentRollout(experimentCR, sourceRollout)
	if err != nil {
		log.Error(err, "Failed to construct desired experiment Rollout")
		r.Recorder.Eventf(experimentCR, corev1.EventTypeWarning, "ConstructionFailed", "Failed to construct experiment Rollout: %s", err.Error())
		r.updateStatusConditions(experimentCR, "ConstructionFailed", fmt.Sprintf("Failed to construct experiment Rollout: %s", err.Error()))
		return nil, err
	}

	// Create or Update experiment Rollout
	return r.createOrUpdateRollout(ctx, experimentCR, desiredExperimentRollout)
}

// Helper function to update status conditions
func (r *ExperimentDeploymentReconciler) updateStatusConditions(experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment, reason, message string) {
	meta.SetStatusCondition(&experimentCR.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeSynced,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
	meta.SetStatusCondition(&experimentCR.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
	experimentCR.Status.ObservedGeneration = experimentCR.Generation
}

func (r *ExperimentDeploymentReconciler) deleteExperimentDeployment(ctx context.Context, namespace, name string) error {
	log := logf.FromContext(ctx)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	// Check if the deployment to be deleted was indeed created for this source.
	// This is a safety check, though owner references should handle most cases.
	// For this example, we'll rely on the name convention.
	// A more robust check might involve labels set by this controller.

	err := r.Delete(ctx, deployment)
	if err != nil && !k8serrors.IsNotFound(err) {
		log.Error(err, "Failed to delete experiment Deployment", "name", name, "namespace", namespace)
		return err
	}
	if k8serrors.IsNotFound(err) {
		log.Info("Experiment Deployment already deleted or not found, nothing to do.", "name", name, "namespace", namespace)
		return nil
	}
	log.Info("Successfully initiated deletion of experiment Deployment", "name", name, "namespace", namespace)
	return nil
}

// createOrUpdateDeployment creates or updates an experiment Deployment
func (r *ExperimentDeploymentReconciler) createOrUpdateDeployment(ctx context.Context, experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment, desired *appsv1.Deployment) (client.Object, error) {
	log := logf.FromContext(ctx)

	experimentToManage := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      desired.Name,
			Namespace: desired.Namespace,
		},
	}

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, experimentToManage, func() error {
		if err := controllerutil.SetControllerReference(experimentCR, experimentToManage, r.Scheme); err != nil {
			return err
		}
		experimentToManage.Spec = desired.Spec
		if experimentToManage.Labels == nil {
			experimentToManage.Labels = make(map[string]string)
		}
		for k, v := range desired.Labels {
			experimentToManage.Labels[k] = v
		}
		if experimentToManage.Annotations == nil {
			experimentToManage.Annotations = make(map[string]string)
		}
		for k, v := range desired.Annotations {
			experimentToManage.Annotations[k] = v
		}
		return nil
	})

	if err != nil {
		log.Error(err, "Failed to create or update experiment Deployment", "name", desired.Name)
		r.Recorder.Eventf(experimentCR, corev1.EventTypeWarning, "UpsertFailed", "Failed to create/update experiment Deployment %s: %s", desired.Name, err.Error())
		r.updateStatusConditions(experimentCR, "UpsertFailed", fmt.Sprintf("Failed to create/update experiment Deployment %s: %s", desired.Name, err.Error()))
		return nil, err
	}

	if opResult != controllerutil.OperationResultNone {
		log.Info("Experiment Deployment successfully reconciled", "operation", opResult, "name", desired.Name)
		r.Recorder.Eventf(experimentCR, corev1.EventTypeNormal, string(opResult), "Experiment Deployment %s %s", desired.Name, opResult)
	}

	return experimentToManage, nil
}

// createOrUpdateStatefulSet creates or updates an experiment StatefulSet
func (r *ExperimentDeploymentReconciler) createOrUpdateStatefulSet(ctx context.Context, experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment, desired *appsv1.StatefulSet) (client.Object, error) {
	log := logf.FromContext(ctx)

	experimentToManage := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      desired.Name,
			Namespace: desired.Namespace,
		},
	}

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, experimentToManage, func() error {
		if err := controllerutil.SetControllerReference(experimentCR, experimentToManage, r.Scheme); err != nil {
			return err
		}
		experimentToManage.Spec = desired.Spec
		if experimentToManage.Labels == nil {
			experimentToManage.Labels = make(map[string]string)
		}
		for k, v := range desired.Labels {
			experimentToManage.Labels[k] = v
		}
		if experimentToManage.Annotations == nil {
			experimentToManage.Annotations = make(map[string]string)
		}
		for k, v := range desired.Annotations {
			experimentToManage.Annotations[k] = v
		}
		return nil
	})

	if err != nil {
		log.Error(err, "Failed to create or update experiment StatefulSet", "name", desired.Name)
		r.Recorder.Eventf(experimentCR, corev1.EventTypeWarning, "UpsertFailed", "Failed to create/update experiment StatefulSet %s: %s", desired.Name, err.Error())
		r.updateStatusConditions(experimentCR, "UpsertFailed", fmt.Sprintf("Failed to create/update experiment StatefulSet %s: %s", desired.Name, err.Error()))
		return nil, err
	}

	if opResult != controllerutil.OperationResultNone {
		log.Info("Experiment StatefulSet successfully reconciled", "operation", opResult, "name", desired.Name)
		r.Recorder.Eventf(experimentCR, corev1.EventTypeNormal, string(opResult), "Experiment StatefulSet %s %s", desired.Name, opResult)
	}

	return experimentToManage, nil
}

// createOrUpdateRollout creates or updates an experiment Rollout
func (r *ExperimentDeploymentReconciler) createOrUpdateRollout(ctx context.Context, experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment, desired *rolloutsv1alpha1.Rollout) (client.Object, error) {
	log := logf.FromContext(ctx)

	experimentToManage := &rolloutsv1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      desired.Name,
			Namespace: desired.Namespace,
		},
	}

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, experimentToManage, func() error {
		if err := controllerutil.SetControllerReference(experimentCR, experimentToManage, r.Scheme); err != nil {
			return err
		}
		experimentToManage.Spec = desired.Spec
		if experimentToManage.Labels == nil {
			experimentToManage.Labels = make(map[string]string)
		}
		for k, v := range desired.Labels {
			experimentToManage.Labels[k] = v
		}
		if experimentToManage.Annotations == nil {
			experimentToManage.Annotations = make(map[string]string)
		}
		for k, v := range desired.Annotations {
			experimentToManage.Annotations[k] = v
		}
		return nil
	})

	if err != nil {
		log.Error(err, "Failed to create or update experiment Rollout", "name", desired.Name)
		r.Recorder.Eventf(experimentCR, corev1.EventTypeWarning, "UpsertFailed", "Failed to create/update experiment Rollout %s: %s", desired.Name, err.Error())
		r.updateStatusConditions(experimentCR, "UpsertFailed", fmt.Sprintf("Failed to create/update experiment Rollout %s: %s", desired.Name, err.Error()))
		return nil, err
	}

	if opResult != controllerutil.OperationResultNone {
		log.Info("Experiment Rollout successfully reconciled", "operation", opResult, "name", desired.Name)
		r.Recorder.Eventf(experimentCR, corev1.EventTypeNormal, string(opResult), "Experiment Rollout %s %s", desired.Name, opResult)
	}

	return experimentToManage, nil
}

func (r *ExperimentDeploymentReconciler) constructExperimentDeployment(
	experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment,
	sourceDeployment *appsv1.Deployment) (*appsv1.Deployment, error) {

	log := logf.FromContext(context.Background()) // Use a background context for logging in helpers

	experimentDeploymentName := experimentCR.Name
	experimentDeploymentNamespace := experimentCR.Namespace // Create in CR's namespace

	// Deep copy the source spec
	experimentSpec := *sourceDeployment.Spec.DeepCopy()

	// Convert source spec to map for merging
	sourceSpecMap := make(map[string]interface{})
	sourceSpecJSON, err := json.Marshal(experimentSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal source deployment spec: %w", err)
	}
	if err := json.Unmarshal(sourceSpecJSON, &sourceSpecMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal source deployment spec to map: %w", err)
	}

	// Convert overrideSpec to map
	overrideSpecMap := make(map[string]interface{})
	if len(experimentCR.Spec.OverrideSpec.Raw) > 0 {
		if err := json.Unmarshal(experimentCR.Spec.OverrideSpec.Raw, &overrideSpecMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal overrideSpec: %w", err)
		}

		// Merge overrideSpec into the sourceSpecMap
		// mergo.WithOverride ensures that overrideSpec values replace sourceSpec values.
		// mergo.WithSliceDeepCopy allows slices to be merged more intelligently
		if err := mergo.Merge(&sourceSpecMap, overrideSpecMap, mergo.WithOverride, mergo.WithSliceDeepCopy); err != nil {
			return nil, fmt.Errorf("failed to merge overrideSpec: %w", err)
		}
	}

	// Convert merged map back to DeploymentSpec
	mergedSpecJSON, err := json.Marshal(sourceSpecMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged spec map: %w", err)
	}
	var finalExperimentSpec appsv1.DeploymentSpec
	if err := json.Unmarshal(mergedSpecJSON, &finalExperimentSpec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal merged spec map to DeploymentSpec: %w", err)
	}

	// Apply replicas from CR spec (takes precedence), otherwise default to 1
	if experimentCR.Spec.Replicas != nil {
		finalExperimentSpec.Replicas = experimentCR.Spec.Replicas
	} else {
		// Default to 1 replica for experiments if not specified in CR
		defaultReplicas := int32(1)
		finalExperimentSpec.Replicas = &defaultReplicas
	}

	// Labels for the experiment deployment's pods
	podLabels := make(map[string]string)
	// Copy labels from source pod template to ensure service discovery
	if sourceDeployment.Spec.Template.Labels != nil {
		for k, v := range sourceDeployment.Spec.Template.Labels {
			podLabels[k] = v
		}
	}
	// Add experiment-specific labels
	podLabels["experiment-controller.example.com/cr-name"] = experimentCR.Name
	podLabels["experiment-controller.example.com/role"] = ExperimentRoleValue
	podLabels["experiment-controller.example.com/source-deployment-name"] = sourceDeployment.Name
	// Ensure these labels are on the pod template
	finalExperimentSpec.Template.ObjectMeta.Labels = podLabels

	// The Deployment's selector must match its pod template labels
	finalExperimentSpec.Selector = &metav1.LabelSelector{
		MatchLabels: podLabels, // Selector matches all pod labels
	}

	// Annotations
	podAnnotations := make(map[string]string)
	if sourceDeployment.Spec.Template.Annotations != nil {
		for k, v := range sourceDeployment.Spec.Template.Annotations {
			podAnnotations[k] = v
		}
	}
	// Add experiment-specific annotations if any
	// podAnnotations["experiment-controller.example.com/some-annotation"] = "value"
	finalExperimentSpec.Template.ObjectMeta.Annotations = podAnnotations

	// Construct the experiment Deployment object
	desiredExperimentDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      experimentDeploymentName,
			Namespace: experimentDeploymentNamespace,
			Labels: map[string]string{ // Labels for the Deployment object itself
				"experiment-controller.example.com/managed-by": "experiment-controller",
				"experiment-controller.example.com/cr-name":    experimentCR.Name,
			},
			Annotations: make(map[string]string), // Add annotations if needed
		},
		Spec: finalExperimentSpec,
	}
	// Copy annotations from CR to experiment deployment object if desired
	// for k, v := range experimentCR.Annotations {
	// desiredExperimentDeployment.Annotations[k] = v
	// }

	log.Info("Constructed desired experiment deployment", "name", desiredExperimentDeployment.Name, "namespace", desiredExperimentDeployment.Namespace, "replicas", *desiredExperimentDeployment.Spec.Replicas)
	return desiredExperimentDeployment, nil
}

func (r *ExperimentDeploymentReconciler) constructExperimentStatefulSet(
	experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment,
	sourceStatefulSet *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {

	log := logf.FromContext(context.Background())

	experimentStatefulSetName := experimentCR.Name
	experimentStatefulSetNamespace := experimentCR.Namespace

	// Deep copy the source spec
	experimentSpec := *sourceStatefulSet.Spec.DeepCopy()

	// Convert source spec to map for merging
	sourceSpecMap := make(map[string]interface{})
	sourceSpecJSON, err := json.Marshal(experimentSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal source statefulset spec: %w", err)
	}
	if err := json.Unmarshal(sourceSpecJSON, &sourceSpecMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal source statefulset spec to map: %w", err)
	}

	// Convert overrideSpec to map
	overrideSpecMap := make(map[string]interface{})
	if len(experimentCR.Spec.OverrideSpec.Raw) > 0 {
		if err := json.Unmarshal(experimentCR.Spec.OverrideSpec.Raw, &overrideSpecMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal overrideSpec: %w", err)
		}

		// Merge overrideSpec into the sourceSpecMap
		if err := mergo.Merge(&sourceSpecMap, overrideSpecMap, mergo.WithOverride, mergo.WithSliceDeepCopy); err != nil {
			return nil, fmt.Errorf("failed to merge overrideSpec: %w", err)
		}
	}

	// Convert merged map back to StatefulSetSpec
	mergedSpecJSON, err := json.Marshal(sourceSpecMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged spec map: %w", err)
	}
	var finalExperimentSpec appsv1.StatefulSetSpec
	if err := json.Unmarshal(mergedSpecJSON, &finalExperimentSpec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal merged spec map to StatefulSetSpec: %w", err)
	}

	// Apply replicas from CR spec (takes precedence)
	if experimentCR.Spec.Replicas != nil {
		finalExperimentSpec.Replicas = experimentCR.Spec.Replicas
	} else if finalExperimentSpec.Replicas == nil {
		defaultReplicas := int32(1)
		finalExperimentSpec.Replicas = &defaultReplicas
	}

	// Labels for the experiment statefulset's pods
	podLabels := make(map[string]string)
	if sourceStatefulSet.Spec.Template.Labels != nil {
		for k, v := range sourceStatefulSet.Spec.Template.Labels {
			podLabels[k] = v
		}
	}
	// Add experiment-specific labels
	podLabels["experiment-controller.example.com/cr-name"] = experimentCR.Name
	podLabels["experiment-controller.example.com/role"] = "experiment"
	podLabels["experiment-controller.example.com/source-statefulset-name"] = sourceStatefulSet.Name
	finalExperimentSpec.Template.ObjectMeta.Labels = podLabels

	// The StatefulSet's selector must match its pod template labels
	finalExperimentSpec.Selector = &metav1.LabelSelector{
		MatchLabels: podLabels,
	}

	// Annotations
	podAnnotations := make(map[string]string)
	if sourceStatefulSet.Spec.Template.Annotations != nil {
		for k, v := range sourceStatefulSet.Spec.Template.Annotations {
			podAnnotations[k] = v
		}
	}
	finalExperimentSpec.Template.ObjectMeta.Annotations = podAnnotations

	// Construct the experiment StatefulSet object
	desiredExperimentStatefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      experimentStatefulSetName,
			Namespace: experimentStatefulSetNamespace,
			Labels: map[string]string{
				"experiment-controller.example.com/managed-by": "experiment-controller",
				"experiment-controller.example.com/cr-name":    experimentCR.Name,
			},
			Annotations: make(map[string]string),
		},
		Spec: finalExperimentSpec,
	}

	log.Info("Constructed desired experiment statefulset", "name", desiredExperimentStatefulSet.Name, "namespace", desiredExperimentStatefulSet.Namespace, "replicas", *desiredExperimentStatefulSet.Spec.Replicas)
	return desiredExperimentStatefulSet, nil
}

func (r *ExperimentDeploymentReconciler) constructExperimentRollout(
	experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment,
	sourceRollout *rolloutsv1alpha1.Rollout) (*rolloutsv1alpha1.Rollout, error) {

	log := logf.FromContext(context.Background())

	experimentRolloutName := experimentCR.Name
	experimentRolloutNamespace := experimentCR.Namespace

	// Deep copy the source spec
	experimentSpec := *sourceRollout.Spec.DeepCopy()

	// Convert source spec to map for merging
	sourceSpecMap := make(map[string]interface{})
	sourceSpecJSON, err := json.Marshal(experimentSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal source rollout spec: %w", err)
	}
	if err := json.Unmarshal(sourceSpecJSON, &sourceSpecMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal source rollout spec to map: %w", err)
	}

	// Convert overrideSpec to map
	overrideSpecMap := make(map[string]interface{})
	if len(experimentCR.Spec.OverrideSpec.Raw) > 0 {
		if err := json.Unmarshal(experimentCR.Spec.OverrideSpec.Raw, &overrideSpecMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal overrideSpec: %w", err)
		}

		// Merge overrideSpec into the sourceSpecMap
		if err := mergo.Merge(&sourceSpecMap, overrideSpecMap, mergo.WithOverride, mergo.WithSliceDeepCopy); err != nil {
			return nil, fmt.Errorf("failed to merge overrideSpec: %w", err)
		}
	}

	// Convert merged map back to RolloutSpec
	mergedSpecJSON, err := json.Marshal(sourceSpecMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged spec map: %w", err)
	}
	var finalExperimentSpec rolloutsv1alpha1.RolloutSpec
	if err := json.Unmarshal(mergedSpecJSON, &finalExperimentSpec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal merged spec map to RolloutSpec: %w", err)
	}

	// Apply replicas from CR spec (takes precedence)
	if experimentCR.Spec.Replicas != nil {
		finalExperimentSpec.Replicas = experimentCR.Spec.Replicas
	} else if finalExperimentSpec.Replicas == nil {
		defaultReplicas := int32(1)
		finalExperimentSpec.Replicas = &defaultReplicas
	}

	// Labels for the experiment rollout's pods
	podLabels := make(map[string]string)
	if sourceRollout.Spec.Template.Labels != nil {
		for k, v := range sourceRollout.Spec.Template.Labels {
			podLabels[k] = v
		}
	}
	// Add experiment-specific labels
	podLabels["experiment-controller.example.com/cr-name"] = experimentCR.Name
	podLabels["experiment-controller.example.com/role"] = "experiment"
	podLabels["experiment-controller.example.com/source-rollout-name"] = sourceRollout.Name
	finalExperimentSpec.Template.ObjectMeta.Labels = podLabels

	// The Rollout's selector must match its pod template labels
	finalExperimentSpec.Selector = &metav1.LabelSelector{
		MatchLabels: podLabels,
	}

	// Annotations
	podAnnotations := make(map[string]string)
	if sourceRollout.Spec.Template.Annotations != nil {
		for k, v := range sourceRollout.Spec.Template.Annotations {
			podAnnotations[k] = v
		}
	}
	finalExperimentSpec.Template.ObjectMeta.Annotations = podAnnotations

	// For Rollouts, we may want to simplify the strategy for experiments
	// Here we keep the original strategy but could override it in the overrideSpec

	// Construct the experiment Rollout object
	desiredExperimentRollout := &rolloutsv1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      experimentRolloutName,
			Namespace: experimentRolloutNamespace,
			Labels: map[string]string{
				"experiment-controller.example.com/managed-by": "experiment-controller",
				"experiment-controller.example.com/cr-name":    experimentCR.Name,
			},
			Annotations: make(map[string]string),
		},
		Spec: finalExperimentSpec,
	}

	log.Info("Constructed desired experiment rollout", "name", desiredExperimentRollout.Name, "namespace", desiredExperimentRollout.Namespace, "replicas", *desiredExperimentRollout.Spec.Replicas)
	return desiredExperimentRollout, nil
}

func (r *ExperimentDeploymentReconciler) updateExperimentWorkloadStatus(
	ctx context.Context,
	experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment,
	experimentWorkload client.Object) (ctrl.Result, error) {

	log := logf.FromContext(ctx)

	// Update status based on workload type
	switch workload := experimentWorkload.(type) {
	case *appsv1.Deployment:
		return r.updateDeploymentStatus(ctx, experimentCR, workload)
	case *appsv1.StatefulSet:
		return r.updateStatefulSetStatus(ctx, experimentCR, workload)
	case *rolloutsv1alpha1.Rollout:
		return r.updateRolloutStatus(ctx, experimentCR, workload)
	default:
		err := fmt.Errorf("unsupported workload type: %T", experimentWorkload)
		log.Error(err, "Unknown workload type for status update")
		return ctrl.Result{}, err
	}
}

func (r *ExperimentDeploymentReconciler) updateDeploymentStatus(
	ctx context.Context,
	experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment,
	expDeployment *appsv1.Deployment) (ctrl.Result, error) {

	log := logf.FromContext(ctx)

	// Fetch the latest state of the experiment deployment to get its status
	currentExpDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: expDeployment.Name, Namespace: expDeployment.Namespace}, currentExpDeployment)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("Experiment deployment not found", "name", expDeployment.Name)
			r.setNotFoundStatus(experimentCR, "Deployment", expDeployment.Name, expDeployment.Namespace)
		} else {
			log.Error(err, "Failed to get current experiment Deployment for status update")
			return ctrl.Result{}, err
		}
	} else {
		experimentCR.Status.ReadyReplicas = currentExpDeployment.Status.ReadyReplicas
		experimentCR.Status.ExperimentResourceRef = &experimentcontrollercomv1alpha1.ExperimentResourceRef{
			Kind:      "Deployment",
			Name:      currentExpDeployment.Name,
			Namespace: currentExpDeployment.Namespace,
		}

		// Determine Ready status
		isReady := currentExpDeployment.Status.ReadyReplicas >= *currentExpDeployment.Spec.Replicas &&
			currentExpDeployment.Status.UpdatedReplicas == *currentExpDeployment.Spec.Replicas &&
			currentExpDeployment.Generation == currentExpDeployment.Status.ObservedGeneration

		if isReady {
			r.setReadyStatus(experimentCR, "Experiment Deployment is Ready")
		} else {
			reason, message := r.getDeploymentNotReadyStatus(currentExpDeployment)
			r.setNotReadyStatus(experimentCR, reason, message)
		}
	}

	return r.finalizeStatusUpdate(ctx, experimentCR)
}

func (r *ExperimentDeploymentReconciler) updateStatefulSetStatus(
	ctx context.Context,
	experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment,
	expStatefulSet *appsv1.StatefulSet) (ctrl.Result, error) {

	log := logf.FromContext(ctx)

	// Fetch the latest state of the experiment statefulset
	currentExpStatefulSet := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: expStatefulSet.Name, Namespace: expStatefulSet.Namespace}, currentExpStatefulSet)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("Experiment statefulset not found", "name", expStatefulSet.Name)
			r.setNotFoundStatus(experimentCR, "StatefulSet", expStatefulSet.Name, expStatefulSet.Namespace)
		} else {
			log.Error(err, "Failed to get current experiment StatefulSet for status update")
			return ctrl.Result{}, err
		}
	} else {
		experimentCR.Status.ReadyReplicas = currentExpStatefulSet.Status.ReadyReplicas
		experimentCR.Status.ExperimentResourceRef = &experimentcontrollercomv1alpha1.ExperimentResourceRef{
			Kind:      "StatefulSet",
			Name:      currentExpStatefulSet.Name,
			Namespace: currentExpStatefulSet.Namespace,
		}

		// Determine Ready status for StatefulSet
		isReady := currentExpStatefulSet.Status.ReadyReplicas >= *currentExpStatefulSet.Spec.Replicas &&
			currentExpStatefulSet.Status.UpdatedReplicas == *currentExpStatefulSet.Spec.Replicas &&
			currentExpStatefulSet.Generation == currentExpStatefulSet.Status.ObservedGeneration

		if isReady {
			r.setReadyStatus(experimentCR, "Experiment StatefulSet is Ready")
		} else {
			r.setNotReadyStatus(experimentCR, "NotReady", "Experiment StatefulSet is not yet ready")
		}
	}

	return r.finalizeStatusUpdate(ctx, experimentCR)
}

func (r *ExperimentDeploymentReconciler) updateRolloutStatus(
	ctx context.Context,
	experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment,
	expRollout *rolloutsv1alpha1.Rollout) (ctrl.Result, error) {

	log := logf.FromContext(ctx)

	// Fetch the latest state of the experiment rollout
	currentExpRollout := &rolloutsv1alpha1.Rollout{}
	err := r.Get(ctx, types.NamespacedName{Name: expRollout.Name, Namespace: expRollout.Namespace}, currentExpRollout)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("Experiment rollout not found", "name", expRollout.Name)
			r.setNotFoundStatus(experimentCR, "Rollout", expRollout.Name, expRollout.Namespace)
		} else {
			log.Error(err, "Failed to get current experiment Rollout for status update")
			return ctrl.Result{}, err
		}
	} else {
		experimentCR.Status.ReadyReplicas = currentExpRollout.Status.ReadyReplicas
		experimentCR.Status.ExperimentResourceRef = &experimentcontrollercomv1alpha1.ExperimentResourceRef{
			Kind:      "Rollout",
			Name:      currentExpRollout.Name,
			Namespace: currentExpRollout.Namespace,
		}

		// Determine Ready status for Rollout
		isReady := currentExpRollout.Status.ReadyReplicas >= *currentExpRollout.Spec.Replicas &&
			currentExpRollout.Status.UpdatedReplicas == *currentExpRollout.Spec.Replicas

		if isReady {
			r.setReadyStatus(experimentCR, "Experiment Rollout is Ready")
		} else {
			r.setNotReadyStatus(experimentCR, "NotReady", "Experiment Rollout is not yet ready")
		}
	}

	return r.finalizeStatusUpdate(ctx, experimentCR)
}

// Helper functions for status management
func (r *ExperimentDeploymentReconciler) setNotFoundStatus(experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment, kind, name, namespace string) {
	meta.SetStatusCondition(&experimentCR.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  "ExperimentWorkloadNotFound",
		Message: fmt.Sprintf("Experiment %s %s/%s not found", kind, namespace, name),
	})
	meta.SetStatusCondition(&experimentCR.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeSynced,
		Status:  metav1.ConditionFalse,
		Reason:  "ExperimentWorkloadNotFound",
		Message: fmt.Sprintf("Experiment %s %s/%s not found", kind, namespace, name),
	})
	experimentCR.Status.ExperimentResourceRef = nil
	experimentCR.Status.ReadyReplicas = 0
}

func (r *ExperimentDeploymentReconciler) setReadyStatus(experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment, message string) {
	meta.SetStatusCondition(&experimentCR.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeReady,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonReconcileSuccess,
		Message: message,
	})
	meta.SetStatusCondition(&experimentCR.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeSynced,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonReconcileSuccess,
		Message: "Experiment workload is Synced",
	})
}

func (r *ExperimentDeploymentReconciler) setNotReadyStatus(experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment, reason, message string) {
	meta.SetStatusCondition(&experimentCR.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
	meta.SetStatusCondition(&experimentCR.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeSynced,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonReconcileSuccess,
		Message: "Experiment workload is Synced, but not yet Ready",
	})
}

func (r *ExperimentDeploymentReconciler) getDeploymentNotReadyStatus(deployment *appsv1.Deployment) (string, string) {
	progressingCondition := getDeploymentCondition(deployment.Status, appsv1.DeploymentProgressing)
	reason := "NotReady"
	message := "Experiment Deployment is not yet ready."
	if progressingCondition != nil && progressingCondition.Status == corev1.ConditionFalse {
		reason = progressingCondition.Reason
		message = progressingCondition.Message
	} else if progressingCondition != nil && progressingCondition.Status == corev1.ConditionTrue && progressingCondition.Reason == "NewReplicaSetAvailable" {
		reason = "Progressing"
		message = "Experiment Deployment is progressing."
	}
	return reason, message
}

func (r *ExperimentDeploymentReconciler) finalizeStatusUpdate(ctx context.Context, experimentCR *experimentcontrollercomv1alpha1.ExperimentDeployment) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	experimentCR.Status.ObservedGeneration = experimentCR.Generation

	if err := r.Status().Update(ctx, experimentCR); err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("ExperimentDeployment not found during status update, possibly already deleted", "name", experimentCR.Name)
			// Still return appropriate result based on current status
			readyCond := meta.FindStatusCondition(experimentCR.Status.Conditions, ConditionTypeReady)
			if readyCond == nil || readyCond.Status == metav1.ConditionFalse {
				return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
			}
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to update ExperimentDeployment status")
		return ctrl.Result{}, err
	}

	// If not ready, requeue to check status again
	readyCond := meta.FindStatusCondition(experimentCR.Status.Conditions, ConditionTypeReady)
	if readyCond == nil || readyCond.Status == metav1.ConditionFalse {
		log.Info("ExperimentDeployment not ready, requeueing for status check.", "name", experimentCR.Name)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// getDeploymentCondition returns the condition with the provided type.
func getDeploymentCondition(status appsv1.DeploymentStatus, condType appsv1.DeploymentConditionType) *appsv1.DeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ExperimentDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("experimentdeployment-controller")
	}

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&experimentcontrollercomv1alpha1.ExperimentDeployment{}).
		Owns(&appsv1.Deployment{}).  // Watch Deployments created by this controller
		Owns(&appsv1.StatefulSet{}). // Watch StatefulSets created by this controller
		Named("experimentdeployment")

	// Only add Rollout watching if Rollouts are available in the cluster
	setupLog := ctrl.Log.WithName("setup")
	if r.isRolloutAvailable(mgr) {
		setupLog.Info("Argo Rollouts detected in cluster, enabling Rollout support")
		builder = builder.Owns(&rolloutsv1alpha1.Rollout{}) // Watch Rollouts created by this controller
	} else {
		setupLog.Info("Argo Rollouts not available in cluster, Rollout support disabled")
	}

	return builder.Complete(r)
}

// isRolloutAvailable checks if Rollouts are available in the cluster
func (r *ExperimentDeploymentReconciler) isRolloutAvailable(mgr ctrl.Manager) bool {
	// First check if the scheme has the Rollout type registered
	if !r.isRolloutSupported() {
		return false
	}

	// Then check if the CRD exists in the cluster
	return r.isRolloutCRDAvailable(mgr)
}

// isRolloutCRDAvailable checks if the Rollout CRD exists in the cluster
func (r *ExperimentDeploymentReconciler) isRolloutCRDAvailable(mgr ctrl.Manager) bool {
	// Try to get the Rollout CRD from the discovery client
	gvk := rolloutsv1alpha1.SchemeGroupVersion.WithKind("Rollout")
	_, err := mgr.GetRESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		setupLog := ctrl.Log.WithName("setup")
		setupLog.Info("Rollout CRD not found in cluster", "error", err)
		return false
	}
	return true
}

// isRolloutSupported checks if Rollouts are supported at runtime
func (r *ExperimentDeploymentReconciler) isRolloutSupported() bool {
	// Check if the scheme has the Rollout type registered
	gvk := rolloutsv1alpha1.SchemeGroupVersion.WithKind("Rollout")
	_, err := r.Scheme.New(gvk)
	if err != nil {
		setupLog := ctrl.Log.WithName("setup")
		setupLog.Info("Rollout type not registered in scheme", "error", err)
		return false
	}
	return true
}
