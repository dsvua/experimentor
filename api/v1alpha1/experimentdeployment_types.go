package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SourceKind defines the kind of the source workload
// +kubebuilder:validation:Enum=Deployment;StatefulSet;Rollout
type SourceKind string

const (
	// SourceKindDeployment represents a Deployment
	SourceKindDeployment SourceKind = "Deployment"
	// SourceKindStatefulSet represents a StatefulSet
	SourceKindStatefulSet SourceKind = "StatefulSet"
	// SourceKindRollout represents an Argo Rollout
	SourceKindRollout SourceKind = "Rollout"
)

// SourceRef defines a reference to the source workload.
type SourceRef struct {
	// Kind specifies the kind of the source workload.
	// Supported kinds are "Deployment", "StatefulSet", "Rollout".
	// +kubebuilder:validation:Required
	Kind SourceKind `json:"kind"`

	// Name is the name of the source workload.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the source workload.
	// If empty, it defaults to the namespace of the ExperimentDeployment CR.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ExperimentDeploymentSpec defines the desired state of ExperimentDeployment
type ExperimentDeploymentSpec struct {
	// SourceRef is a reference to the source workload (Deployment, StatefulSet, or Argo Rollout)
	// from which the experiment will be derived.
	// +kubebuilder:validation:Required
	SourceRef SourceRef `json:"sourceRef"`

	// Replicas is the desired number of replicas for the experiment workload.
	// Defaults to 1 if not specified.
	// For Argo Rollouts, this might translate to a simplified strategy or base replica count.
	// +optional
	// +kubebuilder:default:=1
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`

	// OverrideSpec is a raw JSON/YAML structure representing the partial spec
	// to be deep-merged onto the source workload's spec.
	// The structure should correspond to the 'spec' of the sourceRef.kind.
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	OverrideSpec apiextensionsv1.JSON `json:"overrideSpec"`
}

// ExperimentResourceRef defines a reference to a Kubernetes resource.
type ExperimentResourceRef struct {
	// Kind is the kind of the referenced resource (e.g., Deployment, StatefulSet, Rollout).
	// +optional
	Kind string `json:"kind,omitempty"`

	// Name is the name of the referenced resource.
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace is the namespace of the referenced resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ExperimentDeploymentStatus defines the observed state of ExperimentDeployment
type ExperimentDeploymentStatus struct {
	// Conditions represent the latest available observations of an ExperimentDeployment's state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ExperimentResourceRef is a reference to the managed experiment workload.
	// +optional
	ExperimentResourceRef *ExperimentResourceRef `json:"experimentResourceRef,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ReadyReplicas is the number of ready replicas for the experiment workload.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=experimentdeployments,scope=Namespaced,shortName=expdep,categories=all
// +kubebuilder:printcolumn:name="Source Kind",type="string",JSONPath=".spec.sourceRef.kind"
// +kubebuilder:printcolumn:name="Source Name",type="string",JSONPath=".spec.sourceRef.name"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// ExperimentDeployment is the Schema for the experimentdeployments API
type ExperimentDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExperimentDeploymentSpec   `json:"spec,omitempty"`
	Status ExperimentDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// ExperimentDeploymentList contains a list of ExperimentDeployment
type ExperimentDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExperimentDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExperimentDeployment{}, &ExperimentDeploymentList{})
}
