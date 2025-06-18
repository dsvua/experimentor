package controller

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	experimentcontrollercomv1alpha1 "experimentcontroller.example.com/experiment-deployment/api/v1alpha1"
)

const (
	testNamespace        = "test-namespace"
	testExperimentCRName = "experiment-cr"
)

var _ = Describe("ExperimentDeployment Controller", func() {
	var (
		ctx        context.Context
		cancel     context.CancelFunc
		reconciler *ExperimentDeploymentReconciler
		fakeClient client.Client
		scheme     *runtime.Scheme
		recorder   *record.FakeRecorder
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		scheme = runtime.NewScheme()
		Expect(experimentcontrollercomv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		recorder = record.NewFakeRecorder(100)
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&experimentcontrollercomv1alpha1.ExperimentDeployment{}).Build()
		reconciler = &ExperimentDeploymentReconciler{
			Client:   fakeClient,
			Scheme:   scheme,
			Recorder: recorder,
		}
	})

	AfterEach(func() {
		cancel()
	})

	Context("When reconciling an ExperimentDeployment with Deployment source", func() {
		var (
			namespace            string
			sourceDeploymentName string
			experimentCRName     string
			sourceDeployment     *appsv1.Deployment
			experimentCR         *experimentcontrollercomv1alpha1.ExperimentDeployment
			namespacedName       types.NamespacedName
		)

		BeforeEach(func() {
			namespace = testNamespace
			sourceDeploymentName = "source-deployment"
			experimentCRName = testExperimentCRName
			namespacedName = types.NamespacedName{Name: experimentCRName, Namespace: namespace}

			sourceDeployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sourceDeploymentName,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
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

			experimentCR = &experimentcontrollercomv1alpha1.ExperimentDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      experimentCRName,
					Namespace: namespace,
				},
				Spec: experimentcontrollercomv1alpha1.ExperimentDeploymentSpec{
					SourceRef: experimentcontrollercomv1alpha1.SourceRef{
						Kind: experimentcontrollercomv1alpha1.SourceKindDeployment,
						Name: sourceDeploymentName,
					},
					Replicas:     func() *int32 { r := int32(1); return &r }(),
					OverrideSpec: apiextensionsv1.JSON{Raw: []byte("{}")},
				},
			}
		})

		It("should add finalizer on first reconcile", func() {
			Expect(fakeClient.Create(ctx, sourceDeployment)).To(Succeed())
			Expect(fakeClient.Create(ctx, experimentCR)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			updatedCR := &experimentcontrollercomv1alpha1.ExperimentDeployment{}
			Expect(fakeClient.Get(ctx, namespacedName, updatedCR)).To(Succeed())
			Expect(updatedCR.Finalizers).To(ContainElement(experimentDeploymentFinalizer))
		})

		It("should create experiment deployment successfully", func() {
			Expect(fakeClient.Create(ctx, sourceDeployment)).To(Succeed())
			experimentCR.Finalizers = []string{experimentDeploymentFinalizer}
			Expect(fakeClient.Create(ctx, experimentCR)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(15 * time.Second))

			experimentDeployment := &appsv1.Deployment{}
			expDeploymentName := types.NamespacedName{Name: experimentCRName, Namespace: namespace}
			Expect(fakeClient.Get(ctx, expDeploymentName, experimentDeployment)).To(Succeed())
			Expect(*experimentDeployment.Spec.Replicas).To(Equal(int32(1)))
			Expect(experimentDeployment.Spec.Template.Labels).To(HaveKey("experiment-controller.example.com/cr-name"))
		})

		It("should apply override spec correctly", func() {
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
			experimentCR.Spec.OverrideSpec = apiextensionsv1.JSON{Raw: overrideSpecRaw}
			experimentCR.Finalizers = []string{experimentDeploymentFinalizer}

			Expect(fakeClient.Create(ctx, sourceDeployment)).To(Succeed())
			Expect(fakeClient.Create(ctx, experimentCR)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			experimentDeployment := &appsv1.Deployment{}
			expDeploymentName := types.NamespacedName{Name: experimentCRName, Namespace: namespace}
			Expect(fakeClient.Get(ctx, expDeploymentName, experimentDeployment)).To(Succeed())
			Expect(experimentDeployment.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:1.16"))
		})

		It("should handle missing source deployment", func() {
			experimentCR.Finalizers = []string{experimentDeploymentFinalizer}
			Expect(fakeClient.Create(ctx, experimentCR)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))

			updatedCR := &experimentcontrollercomv1alpha1.ExperimentDeployment{}
			Expect(fakeClient.Get(ctx, namespacedName, updatedCR)).To(Succeed())

			syncedCondition := meta.FindStatusCondition(updatedCR.Status.Conditions, ConditionTypeSynced)
			Expect(syncedCondition).NotTo(BeNil())
			Expect(syncedCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(syncedCondition.Reason).To(Equal("SourceNotFound"))
		})

		It("should handle unsupported source kind", func() {
			experimentCR.Spec.SourceRef.Kind = "UnsupportedKind"
			experimentCR.Finalizers = []string{experimentDeploymentFinalizer}
			Expect(fakeClient.Create(ctx, experimentCR)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).To(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			updatedCR := &experimentcontrollercomv1alpha1.ExperimentDeployment{}
			Expect(fakeClient.Get(ctx, namespacedName, updatedCR)).To(Succeed())

			syncedCondition := meta.FindStatusCondition(updatedCR.Status.Conditions, ConditionTypeSynced)
			Expect(syncedCondition).NotTo(BeNil())
			Expect(syncedCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(syncedCondition.Reason).To(Equal("ValidationFailed"))
		})

		It("should handle deletion correctly", func() {
			Expect(fakeClient.Create(ctx, sourceDeployment)).To(Succeed())
			experimentCR.Finalizers = []string{experimentDeploymentFinalizer}
			Expect(fakeClient.Create(ctx, experimentCR)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			experimentDeployment := &appsv1.Deployment{}
			expDeploymentName := types.NamespacedName{Name: experimentCRName, Namespace: namespace}
			Expect(fakeClient.Get(ctx, expDeploymentName, experimentDeployment)).To(Succeed())

			Expect(fakeClient.Delete(ctx, experimentCR)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			err = fakeClient.Get(ctx, expDeploymentName, experimentDeployment)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should handle invalid override spec", func() {
			experimentCR.Spec.OverrideSpec = apiextensionsv1.JSON{Raw: []byte("{invalid json")}
			experimentCR.Finalizers = []string{experimentDeploymentFinalizer}

			Expect(fakeClient.Create(ctx, sourceDeployment)).To(Succeed())
			Expect(fakeClient.Create(ctx, experimentCR)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).To(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	Context("When reconciling an ExperimentDeployment with StatefulSet source", func() {
		var (
			namespace             string
			sourceStatefulSetName string
			experimentCRName      string
			sourceStatefulSet     *appsv1.StatefulSet
			experimentCR          *experimentcontrollercomv1alpha1.ExperimentDeployment
			namespacedName        types.NamespacedName
		)

		BeforeEach(func() {
			namespace = testNamespace
			sourceStatefulSetName = "source-statefulset"
			experimentCRName = testExperimentCRName
			namespacedName = types.NamespacedName{Name: experimentCRName, Namespace: namespace}

			sourceStatefulSet = &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sourceStatefulSetName,
					Namespace: namespace,
				},
				Spec: appsv1.StatefulSetSpec{
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
					ServiceName: "test-service",
				},
			}

			experimentCR = &experimentcontrollercomv1alpha1.ExperimentDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      experimentCRName,
					Namespace: namespace,
				},
				Spec: experimentcontrollercomv1alpha1.ExperimentDeploymentSpec{
					SourceRef: experimentcontrollercomv1alpha1.SourceRef{
						Kind: experimentcontrollercomv1alpha1.SourceKindStatefulSet,
						Name: sourceStatefulSetName,
					},
					Replicas:     func() *int32 { r := int32(1); return &r }(),
					OverrideSpec: apiextensionsv1.JSON{Raw: []byte("{}")},
				},
			}
		})

		It("should create experiment statefulset successfully", func() {
			Expect(fakeClient.Create(ctx, sourceStatefulSet)).To(Succeed())
			experimentCR.Finalizers = []string{experimentDeploymentFinalizer}
			Expect(fakeClient.Create(ctx, experimentCR)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(15 * time.Second))

			experimentStatefulSet := &appsv1.StatefulSet{}
			expStatefulSetName := types.NamespacedName{Name: experimentCRName, Namespace: namespace}
			Expect(fakeClient.Get(ctx, expStatefulSetName, experimentStatefulSet)).To(Succeed())
			Expect(*experimentStatefulSet.Spec.Replicas).To(Equal(int32(1)))
			Expect(experimentStatefulSet.Spec.Template.Labels).To(HaveKey("experiment-controller.example.com/cr-name"))
		})
	})

	Context("When testing status updates", func() {
		var (
			namespace        string
			experimentCRName string
			experimentCR     *experimentcontrollercomv1alpha1.ExperimentDeployment
		)

		BeforeEach(func() {
			namespace = testNamespace
			experimentCRName = testExperimentCRName

			experimentCR = &experimentcontrollercomv1alpha1.ExperimentDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      experimentCRName,
					Namespace: namespace,
				},
				Spec: experimentcontrollercomv1alpha1.ExperimentDeploymentSpec{
					SourceRef: experimentcontrollercomv1alpha1.SourceRef{
						Kind: experimentcontrollercomv1alpha1.SourceKindDeployment,
						Name: "source-deployment",
					},
				},
			}
		})

		It("should update status to ready when deployment is ready", func() {
			experimentDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      experimentCRName,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: func() *int32 { r := int32(1); return &r }(),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas:      1,
					UpdatedReplicas:    1,
					ObservedGeneration: 1,
				},
			}
			experimentDeployment.Generation = 1

			Expect(fakeClient.Create(ctx, experimentDeployment)).To(Succeed())

			result, err := reconciler.updateDeploymentStatus(ctx, experimentCR, experimentDeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			readyCondition := meta.FindStatusCondition(experimentCR.Status.Conditions, ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should update status to not ready when deployment is not ready", func() {
			experimentDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      experimentCRName,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: func() *int32 { r := int32(2); return &r }(),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas:   1,
					UpdatedReplicas: 1,
				},
			}

			Expect(fakeClient.Create(ctx, experimentDeployment)).To(Succeed())

			result, err := reconciler.updateDeploymentStatus(ctx, experimentCR, experimentDeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(15 * time.Second))

			readyCondition := meta.FindStatusCondition(experimentCR.Status.Conditions, ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("When testing helper functions", func() {
		It("should handle construct experiment deployment with defaults", func() {
			sourceDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source-deployment",
					Namespace: testNamespace,
				},
				Spec: appsv1.DeploymentSpec{
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
					Name:      testExperimentCRName,
					Namespace: testNamespace,
				},
				Spec: experimentcontrollercomv1alpha1.ExperimentDeploymentSpec{
					SourceRef: experimentcontrollercomv1alpha1.SourceRef{
						Kind: experimentcontrollercomv1alpha1.SourceKindDeployment,
						Name: "source-deployment",
					},
				},
			}

			result, err := reconciler.constructExperimentDeployment(experimentCR, sourceDeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result.Spec.Replicas).To(Equal(int32(1)))
			Expect(result.Spec.Template.Labels).To(HaveKey("experiment-controller.example.com/cr-name"))
			Expect(result.Spec.Template.Labels["experiment-controller.example.com/role"]).To(Equal(ExperimentRoleValue))
		})

		It("should handle deleteExperimentDeployment when deployment exists", func() {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: testNamespace,
				},
			}
			Expect(fakeClient.Create(ctx, deployment)).To(Succeed())

			err := reconciler.deleteExperimentDeployment(ctx, testNamespace, "test-deployment")
			Expect(err).NotTo(HaveOccurred())

			err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-deployment", Namespace: testNamespace}, deployment)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should handle deleteExperimentDeployment when deployment does not exist", func() {
			err := reconciler.deleteExperimentDeployment(ctx, testNamespace, "non-existent-deployment")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When testing cross-namespace sources", func() {
		It("should handle source in different namespace", func() {
			sourceNamespace := "source-namespace"
			experimentNamespace := "experiment-namespace"

			sourceDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source-deployment",
					Namespace: sourceNamespace,
				},
				Spec: appsv1.DeploymentSpec{
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
					Name:      testExperimentCRName,
					Namespace: experimentNamespace,
				},
				Spec: experimentcontrollercomv1alpha1.ExperimentDeploymentSpec{
					SourceRef: experimentcontrollercomv1alpha1.SourceRef{
						Kind:      experimentcontrollercomv1alpha1.SourceKindDeployment,
						Name:      "source-deployment",
						Namespace: sourceNamespace,
					},
					Replicas:     func() *int32 { r := int32(1); return &r }(),
					OverrideSpec: apiextensionsv1.JSON{Raw: []byte("{}")},
				},
			}
			experimentCR.Finalizers = []string{experimentDeploymentFinalizer}

			Expect(fakeClient.Create(ctx, sourceDeployment)).To(Succeed())
			Expect(fakeClient.Create(ctx, experimentCR)).To(Succeed())

			namespacedName := types.NamespacedName{Name: testExperimentCRName, Namespace: experimentNamespace}
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(15 * time.Second))

			experimentDeployment := &appsv1.Deployment{}
			expDeploymentName := types.NamespacedName{Name: testExperimentCRName, Namespace: experimentNamespace}
			Expect(fakeClient.Get(ctx, expDeploymentName, experimentDeployment)).To(Succeed())
			Expect(*experimentDeployment.Spec.Replicas).To(Equal(int32(1)))
		})
	})

	Context("When testing reconcile result not found", func() {
		It("should return nil when ExperimentDeployment is not found", func() {
			namespacedName := types.NamespacedName{Name: "non-existent", Namespace: testNamespace}
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})
})
