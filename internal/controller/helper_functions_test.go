package controller

import (
	"context"
	"encoding/json"
	"testing"

	rolloutsv1alpha1 "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	experimentcontrollercomv1alpha1 "experimentcontroller.example.com/experiment-deployment/api/v1alpha1"
)

func TestHelperFunctions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Helper Functions Suite")
}

var _ = Describe("Helper Functions", func() {
	var (
		ctx        context.Context
		reconciler *ExperimentDeploymentReconciler
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(experimentcontrollercomv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(rolloutsv1alpha1.AddToScheme(scheme)).To(Succeed())

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler = &ExperimentDeploymentReconciler{
			Client:   fakeClient,
			Scheme:   scheme,
			Recorder: record.NewFakeRecorder(100),
		}
	})

	Context("constructExperimentStatefulSet", func() {
		It("should construct experiment StatefulSet correctly", func() {
			sourceStatefulSet := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source-statefulset",
					Namespace: "test-namespace",
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas:    func() *int32 { r := int32(3); return &r }(),
					ServiceName: "test-service",
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "source-app"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "source-app"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "nginx:1.14",
								},
							},
						},
					},
				},
			}

			experimentCR := &experimentcontrollercomv1alpha1.ExperimentDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "experiment-cr",
					Namespace: "test-namespace",
				},
				Spec: experimentcontrollercomv1alpha1.ExperimentDeploymentSpec{
					SourceRef: experimentcontrollercomv1alpha1.SourceRef{
						Kind: experimentcontrollercomv1alpha1.SourceKindStatefulSet,
						Name: "source-statefulset",
					},
					Replicas: func() *int32 { r := int32(1); return &r }(),
				},
			}

			result, err := reconciler.constructExperimentStatefulSet(experimentCR, sourceStatefulSet)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result.Spec.Replicas).To(Equal(int32(1)))
			Expect(result.Spec.ServiceName).To(Equal("test-service"))
			Expect(result.Spec.Template.Labels).To(HaveKey("experiment-controller.example.com/cr-name"))
			Expect(result.Spec.Template.Labels["experiment-controller.example.com/role"]).To(Equal("experiment"))
		})

		It("should handle override spec for StatefulSet", func() {
			sourceStatefulSet := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source-statefulset",
					Namespace: "test-namespace",
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas:    func() *int32 { r := int32(3); return &r }(),
					ServiceName: "test-service",
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "source-app"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "source-app"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "nginx:1.14",
								},
							},
						},
					},
				},
			}

			overrideSpec := map[string]interface{}{
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
			overrideSpecRaw, _ := json.Marshal(overrideSpec)

			experimentCR := &experimentcontrollercomv1alpha1.ExperimentDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "experiment-cr",
					Namespace: "test-namespace",
				},
				Spec: experimentcontrollercomv1alpha1.ExperimentDeploymentSpec{
					SourceRef: experimentcontrollercomv1alpha1.SourceRef{
						Kind: experimentcontrollercomv1alpha1.SourceKindStatefulSet,
						Name: "source-statefulset",
					},
					OverrideSpec: apiextensionsv1.JSON{Raw: overrideSpecRaw},
				},
			}

			result, err := reconciler.constructExperimentStatefulSet(experimentCR, sourceStatefulSet)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:1.16"))
		})
	})

	Context("constructExperimentRollout", func() {
		It("should construct experiment Rollout correctly", func() {
			sourceRollout := &rolloutsv1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source-rollout",
					Namespace: "test-namespace",
				},
				Spec: rolloutsv1alpha1.RolloutSpec{
					Replicas: func() *int32 { r := int32(3); return &r }(),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "source-app"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "source-app"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "nginx:1.14",
								},
							},
						},
					},
				},
			}

			experimentCR := &experimentcontrollercomv1alpha1.ExperimentDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "experiment-cr",
					Namespace: "test-namespace",
				},
				Spec: experimentcontrollercomv1alpha1.ExperimentDeploymentSpec{
					SourceRef: experimentcontrollercomv1alpha1.SourceRef{
						Kind: experimentcontrollercomv1alpha1.SourceKindRollout,
						Name: "source-rollout",
					},
					Replicas: func() *int32 { r := int32(1); return &r }(),
				},
			}

			result, err := reconciler.constructExperimentRollout(experimentCR, sourceRollout)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result.Spec.Replicas).To(Equal(int32(1)))
			Expect(result.Spec.Template.Labels).To(HaveKey("experiment-controller.example.com/cr-name"))
			Expect(result.Spec.Template.Labels["experiment-controller.example.com/role"]).To(Equal("experiment"))
		})
	})

	Context("updateStatusConditions", func() {
		It("should set error conditions correctly", func() {
			experimentCR := &experimentcontrollercomv1alpha1.ExperimentDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-experiment",
					Namespace: "test-namespace",
				},
			}

			reconciler.updateStatusConditions(experimentCR, "TestReason", "Test error message")

			syncedCondition := meta.FindStatusCondition(experimentCR.Status.Conditions, ConditionTypeSynced)
			Expect(syncedCondition).NotTo(BeNil())
			Expect(syncedCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(syncedCondition.Reason).To(Equal("TestReason"))
			Expect(syncedCondition.Message).To(Equal("Test error message"))

			readyCondition := meta.FindStatusCondition(experimentCR.Status.Conditions, ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal("TestReason"))
		})
	})

	Context("setReadyStatus", func() {
		It("should set ready status correctly", func() {
			experimentCR := &experimentcontrollercomv1alpha1.ExperimentDeployment{}

			reconciler.setReadyStatus(experimentCR, "Test ready message")

			readyCondition := meta.FindStatusCondition(experimentCR.Status.Conditions, ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCondition.Reason).To(Equal(ReasonReconcileSuccess))
			Expect(readyCondition.Message).To(Equal("Test ready message"))

			syncedCondition := meta.FindStatusCondition(experimentCR.Status.Conditions, ConditionTypeSynced)
			Expect(syncedCondition).NotTo(BeNil())
			Expect(syncedCondition.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("setNotReadyStatus", func() {
		It("should set not ready status correctly", func() {
			experimentCR := &experimentcontrollercomv1alpha1.ExperimentDeployment{}

			reconciler.setNotReadyStatus(experimentCR, "TestNotReadyReason", "Test not ready message")

			readyCondition := meta.FindStatusCondition(experimentCR.Status.Conditions, ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal("TestNotReadyReason"))
			Expect(readyCondition.Message).To(Equal("Test not ready message"))

			syncedCondition := meta.FindStatusCondition(experimentCR.Status.Conditions, ConditionTypeSynced)
			Expect(syncedCondition).NotTo(BeNil())
			Expect(syncedCondition.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("setNotFoundStatus", func() {
		It("should set not found status correctly", func() {
			experimentCR := &experimentcontrollercomv1alpha1.ExperimentDeployment{}

			reconciler.setNotFoundStatus(experimentCR, "Deployment", "test-deployment", "test-namespace")

			readyCondition := meta.FindStatusCondition(experimentCR.Status.Conditions, ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal("ExperimentWorkloadNotFound"))

			syncedCondition := meta.FindStatusCondition(experimentCR.Status.Conditions, ConditionTypeSynced)
			Expect(syncedCondition).NotTo(BeNil())
			Expect(syncedCondition.Status).To(Equal(metav1.ConditionFalse))

			Expect(experimentCR.Status.ExperimentResourceRef).To(BeNil())
			Expect(experimentCR.Status.ReadyReplicas).To(Equal(int32(0)))
		})
	})

	Context("getDeploymentNotReadyStatus", func() {
		It("should return correct status for progressing deployment", func() {
			deployment := &appsv1.Deployment{
				Status: appsv1.DeploymentStatus{
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:   appsv1.DeploymentProgressing,
							Status: corev1.ConditionTrue,
							Reason: "NewReplicaSetAvailable",
						},
					},
				},
			}

			reason, message := reconciler.getDeploymentNotReadyStatus(deployment)
			Expect(reason).To(Equal("Progressing"))
			Expect(message).To(Equal("Experiment Deployment is progressing."))
		})

		It("should return correct status for failed deployment", func() {
			deployment := &appsv1.Deployment{
				Status: appsv1.DeploymentStatus{
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:    appsv1.DeploymentProgressing,
							Status:  corev1.ConditionFalse,
							Reason:  "ProgressDeadlineExceeded",
							Message: "Deployment has exceeded its progress deadline",
						},
					},
				},
			}

			reason, message := reconciler.getDeploymentNotReadyStatus(deployment)
			Expect(reason).To(Equal("ProgressDeadlineExceeded"))
			Expect(message).To(Equal("Deployment has exceeded its progress deadline"))
		})

		It("should return default status when no conditions", func() {
			deployment := &appsv1.Deployment{}

			reason, message := reconciler.getDeploymentNotReadyStatus(deployment)
			Expect(reason).To(Equal("NotReady"))
			Expect(message).To(Equal("Experiment Deployment is not yet ready."))
		})
	})

	Context("isRolloutSupported", func() {
		It("should return true when rollout is supported", func() {
			supported := reconciler.isRolloutSupported()
			Expect(supported).To(BeTrue())
		})
	})

	Context("updateExperimentWorkloadStatus", func() {
		It("should handle unsupported workload type", func() {
			experimentCR := &experimentcontrollercomv1alpha1.ExperimentDeployment{}
			unsupportedWorkload := &corev1.Pod{}

			result, err := reconciler.updateExperimentWorkloadStatus(ctx, experimentCR, unsupportedWorkload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported workload type"))
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})
})
