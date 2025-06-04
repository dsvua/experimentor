package v1alpha1

import (
	"encoding/json"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestV1alpha1Types(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "V1alpha1 API Types Suite")
}

var _ = Describe("ExperimentDeployment API Types", func() {
	Context("SourceKind constants", func() {
		It("should have correct string values", func() {
			Expect(string(SourceKindDeployment)).To(Equal("Deployment"))
			Expect(string(SourceKindStatefulSet)).To(Equal("StatefulSet"))
			Expect(string(SourceKindRollout)).To(Equal("Rollout"))
		})
	})

	Context("SourceRef", func() {
		It("should create valid SourceRef", func() {
			sourceRef := SourceRef{
				Kind:      SourceKindDeployment,
				Name:      "test-deployment",
				Namespace: "test-namespace",
			}

			Expect(sourceRef.Kind).To(Equal(SourceKindDeployment))
			Expect(sourceRef.Name).To(Equal("test-deployment"))
			Expect(sourceRef.Namespace).To(Equal("test-namespace"))
		})

		It("should handle empty namespace", func() {
			sourceRef := SourceRef{
				Kind: SourceKindDeployment,
				Name: "test-deployment",
			}

			Expect(sourceRef.Namespace).To(BeEmpty())
		})
	})

	Context("ExperimentDeploymentSpec", func() {
		It("should create valid spec with required fields", func() {
			overrideSpec := map[string]interface{}{
				"replicas": 2,
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "test-container",
								"image": "nginx:1.16",
							},
						},
					},
				},
			}
			overrideSpecRaw, err := json.Marshal(overrideSpec)
			Expect(err).NotTo(HaveOccurred())

			spec := ExperimentDeploymentSpec{
				SourceRef: SourceRef{
					Kind: SourceKindDeployment,
					Name: "test-deployment",
				},
				Replicas:     func() *int32 { r := int32(1); return &r }(),
				OverrideSpec: apiextensionsv1.JSON{Raw: overrideSpecRaw},
			}

			Expect(spec.SourceRef.Kind).To(Equal(SourceKindDeployment))
			Expect(spec.SourceRef.Name).To(Equal("test-deployment"))
			Expect(*spec.Replicas).To(Equal(int32(1)))
			Expect(spec.OverrideSpec.Raw).NotTo(BeEmpty())
		})

		It("should handle nil replicas", func() {
			spec := ExperimentDeploymentSpec{
				SourceRef: SourceRef{
					Kind: SourceKindDeployment,
					Name: "test-deployment",
				},
				OverrideSpec: apiextensionsv1.JSON{Raw: []byte("{}")},
			}

			Expect(spec.Replicas).To(BeNil())
		})

		It("should handle empty override spec", func() {
			spec := ExperimentDeploymentSpec{
				SourceRef: SourceRef{
					Kind: SourceKindDeployment,
					Name: "test-deployment",
				},
				OverrideSpec: apiextensionsv1.JSON{Raw: []byte("{}")},
			}

			Expect(string(spec.OverrideSpec.Raw)).To(Equal("{}"))
		})
	})

	Context("ExperimentResourceRef", func() {
		It("should create valid resource reference", func() {
			ref := ExperimentResourceRef{
				Kind:      "Deployment",
				Name:      "experiment-deployment",
				Namespace: "test-namespace",
			}

			Expect(ref.Kind).To(Equal("Deployment"))
			Expect(ref.Name).To(Equal("experiment-deployment"))
			Expect(ref.Namespace).To(Equal("test-namespace"))
		})

		It("should handle empty fields", func() {
			ref := ExperimentResourceRef{}

			Expect(ref.Kind).To(BeEmpty())
			Expect(ref.Name).To(BeEmpty())
			Expect(ref.Namespace).To(BeEmpty())
		})
	})

	Context("ExperimentDeploymentStatus", func() {
		It("should create valid status", func() {
			status := ExperimentDeploymentStatus{
				Conditions: []metav1.Condition{
					{
						Type:   "Ready",
						Status: metav1.ConditionTrue,
						Reason: "ReconcileSuccess",
					},
				},
				ExperimentResourceRef: &ExperimentResourceRef{
					Kind:      "Deployment",
					Name:      "experiment-deployment",
					Namespace: "test-namespace",
				},
				ObservedGeneration: 1,
				ReadyReplicas:      2,
			}

			Expect(status.Conditions).To(HaveLen(1))
			Expect(status.Conditions[0].Type).To(Equal("Ready"))
			Expect(status.ExperimentResourceRef).NotTo(BeNil())
			Expect(status.ObservedGeneration).To(Equal(int64(1)))
			Expect(status.ReadyReplicas).To(Equal(int32(2)))
		})

		It("should handle conditions manipulation", func() {
			status := ExperimentDeploymentStatus{}

			condition := metav1.Condition{
				Type:   "Ready",
				Status: metav1.ConditionFalse,
				Reason: "NotReady",
			}

			meta.SetStatusCondition(&status.Conditions, condition)
			Expect(status.Conditions).To(HaveLen(1))

			readyCondition := meta.FindStatusCondition(status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("ExperimentDeployment", func() {
		It("should create complete ExperimentDeployment", func() {
			experiment := ExperimentDeployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ExperimentDeployment",
					APIVersion: "experimentcontroller.example.com/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-experiment",
					Namespace: "test-namespace",
				},
				Spec: ExperimentDeploymentSpec{
					SourceRef: SourceRef{
						Kind: SourceKindDeployment,
						Name: "source-deployment",
					},
					Replicas:     func() *int32 { r := int32(1); return &r }(),
					OverrideSpec: apiextensionsv1.JSON{Raw: []byte("{}")},
				},
				Status: ExperimentDeploymentStatus{
					ObservedGeneration: 1,
					ReadyReplicas:      1,
				},
			}

			Expect(experiment.Kind).To(Equal("ExperimentDeployment"))
			Expect(experiment.Name).To(Equal("test-experiment"))
			Expect(experiment.Namespace).To(Equal("test-namespace"))
			Expect(experiment.Spec.SourceRef.Kind).To(Equal(SourceKindDeployment))
			Expect(*experiment.Spec.Replicas).To(Equal(int32(1)))
		})

		It("should support all source kinds", func() {
			testCases := []struct {
				name       string
				sourceKind SourceKind
			}{
				{"Deployment", SourceKindDeployment},
				{"StatefulSet", SourceKindStatefulSet},
				{"Rollout", SourceKindRollout},
			}

			for _, tc := range testCases {
				experiment := ExperimentDeployment{
					Spec: ExperimentDeploymentSpec{
						SourceRef: SourceRef{
							Kind: tc.sourceKind,
							Name: "test-source",
						},
						OverrideSpec: apiextensionsv1.JSON{Raw: []byte("{}")},
					},
				}

				Expect(experiment.Spec.SourceRef.Kind).To(Equal(tc.sourceKind))
				Expect(string(experiment.Spec.SourceRef.Kind)).To(Equal(tc.name))
			}
		})
	})

	Context("ExperimentDeploymentList", func() {
		It("should create valid list", func() {
			list := ExperimentDeploymentList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ExperimentDeploymentList",
					APIVersion: "experimentcontroller.example.com/v1alpha1",
				},
				Items: []ExperimentDeployment{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "experiment-1",
						},
						Spec: ExperimentDeploymentSpec{
							SourceRef: SourceRef{
								Kind: SourceKindDeployment,
								Name: "source-1",
							},
							OverrideSpec: apiextensionsv1.JSON{Raw: []byte("{}")},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "experiment-2",
						},
						Spec: ExperimentDeploymentSpec{
							SourceRef: SourceRef{
								Kind: SourceKindStatefulSet,
								Name: "source-2",
							},
							OverrideSpec: apiextensionsv1.JSON{Raw: []byte("{}")},
						},
					},
				},
			}

			Expect(list.Kind).To(Equal("ExperimentDeploymentList"))
			Expect(list.Items).To(HaveLen(2))
			Expect(list.Items[0].Name).To(Equal("experiment-1"))
			Expect(list.Items[1].Name).To(Equal("experiment-2"))
		})

		It("should handle empty list", func() {
			list := ExperimentDeploymentList{}
			Expect(list.Items).To(BeEmpty())
		})
	})

	Context("JSON serialization", func() {
		It("should serialize and deserialize SourceRef correctly", func() {
			original := SourceRef{
				Kind:      SourceKindRollout,
				Name:      "test-rollout",
				Namespace: "test-ns",
			}

			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var deserialized SourceRef
			err = json.Unmarshal(data, &deserialized)
			Expect(err).NotTo(HaveOccurred())

			Expect(deserialized).To(Equal(original))
		})

		It("should serialize and deserialize ExperimentDeployment correctly", func() {
			original := ExperimentDeployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ExperimentDeployment",
					APIVersion: "experimentcontroller.example.com/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-experiment",
					Namespace: "test-namespace",
				},
				Spec: ExperimentDeploymentSpec{
					SourceRef: SourceRef{
						Kind: SourceKindDeployment,
						Name: "source-deployment",
					},
					Replicas:     func() *int32 { r := int32(1); return &r }(),
					OverrideSpec: apiextensionsv1.JSON{Raw: []byte(`{"replicas": 2}`)},
				},
			}

			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var deserialized ExperimentDeployment
			err = json.Unmarshal(data, &deserialized)
			Expect(err).NotTo(HaveOccurred())

			Expect(deserialized.Name).To(Equal(original.Name))
			Expect(deserialized.Spec.SourceRef.Kind).To(Equal(original.Spec.SourceRef.Kind))
			Expect(*deserialized.Spec.Replicas).To(Equal(*original.Spec.Replicas))
		})
	})

	Context("Runtime Object interface", func() {
		It("should implement runtime.Object interface", func() {
			experiment := &ExperimentDeployment{}
			var obj runtime.Object = experiment
			Expect(obj).NotTo(BeNil())

			gvk := experiment.GetObjectKind().GroupVersionKind()
			Expect(gvk.Group).To(BeEmpty())
			Expect(gvk.Version).To(BeEmpty())
			Expect(gvk.Kind).To(BeEmpty())

			experiment.SetGroupVersionKind(GroupVersion.WithKind("ExperimentDeployment"))
			gvk = experiment.GetObjectKind().GroupVersionKind()
			Expect(gvk.Group).To(Equal("experimentcontroller.example.com"))
			Expect(gvk.Version).To(Equal("v1alpha1"))
			Expect(gvk.Kind).To(Equal("ExperimentDeployment"))
		})

		It("should deep copy correctly", func() {
			original := &ExperimentDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-experiment",
					Namespace: "test-namespace",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: ExperimentDeploymentSpec{
					SourceRef: SourceRef{
						Kind: SourceKindDeployment,
						Name: "source-deployment",
					},
					Replicas:     func() *int32 { r := int32(1); return &r }(),
					OverrideSpec: apiextensionsv1.JSON{Raw: []byte("{}")},
				},
			}

			copy := original.DeepCopy()
			Expect(copy).To(Equal(original))
			Expect(copy).NotTo(BeIdenticalTo(original))

			copy.Name = "different-name"
			Expect(copy.Name).To(Equal("different-name"))
			Expect(original.Name).To(Equal("test-experiment"))
		})
	})
})
