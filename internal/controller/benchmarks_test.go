package controller

import (
	"context"
	"encoding/json"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	experimentcontrollercomv1alpha1 "experimentcontroller.example.com/experiment-deployment/api/v1alpha1"
)

func BenchmarkConstructExperimentDeployment(b *testing.B) {
	scheme := runtime.NewScheme()
	_ = experimentcontrollercomv1alpha1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := &ExperimentDeploymentReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(100),
	}

	sourceDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "source-deployment",
			Namespace: "test-namespace",
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

	overrideSpec := map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "test-container",
						"image": "nginx:1.16",
						"env": []interface{}{
							map[string]interface{}{
								"name":  "TEST_ENV",
								"value": "test-value",
							},
						},
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
				Kind: experimentcontrollercomv1alpha1.SourceKindDeployment,
				Name: "source-deployment",
			},
			Replicas:     func() *int32 { r := int32(1); return &r }(),
			OverrideSpec: apiextensionsv1.JSON{Raw: overrideSpecRaw},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := reconciler.constructExperimentDeployment(experimentCR, sourceDeployment)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConstructExperimentDeploymentLargeSpec(b *testing.B) {
	scheme := runtime.NewScheme()
	_ = experimentcontrollercomv1alpha1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := &ExperimentDeploymentReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(100),
	}

	// Create a larger, more complex source deployment
	sourceDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "source-deployment",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app":     "source-app",
				"version": "v1.0.0",
				"tier":    "frontend",
			},
			Annotations: map[string]string{
				"deployment.kubernetes.io/revision":                "1",
				"kubectl.kubernetes.io/last-applied-configuration": "{}",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: func() *int32 { r := int32(5); return &r }(),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":     "source-app",
					"version": "v1.0.0",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":     "source-app",
						"version": "v1.0.0",
					},
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/port":   "8080",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main-container",
							Image: "nginx:1.14",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 80, Name: "http"},
								{ContainerPort: 8080, Name: "metrics"},
							},
							Env: []corev1.EnvVar{
								{Name: "ENV", Value: "production"},
								{Name: "LOG_LEVEL", Value: "info"},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
						},
						{
							Name:    "sidecar",
							Image:   "busybox:1.35",
							Command: []string{"/bin/sh", "-c", "while true; do echo hello; sleep 10; done"},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:    "init-container",
							Image:   "busybox:1.35",
							Command: []string{"/bin/sh", "-c", "echo init complete"},
						},
					},
				},
			},
		},
	}

	// Large override spec
	overrideSpec := map[string]interface{}{
		"template": map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"experiment.example.com/enabled": "true",
				},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main-container",
						"image": "nginx:1.16",
						"env": []interface{}{
							map[string]interface{}{
								"name":  "ENV",
								"value": "experiment",
							},
							map[string]interface{}{
								"name":  "EXPERIMENT_ID",
								"value": "exp-001",
							},
						},
					},
					map[string]interface{}{
						"name":  "sidecar",
						"image": "busybox:1.36",
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
				Kind: experimentcontrollercomv1alpha1.SourceKindDeployment,
				Name: "source-deployment",
			},
			Replicas:     func() *int32 { r := int32(2); return &r }(),
			OverrideSpec: apiextensionsv1.JSON{Raw: overrideSpecRaw},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := reconciler.constructExperimentDeployment(experimentCR, sourceDeployment)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReconcileHappyPath(b *testing.B) {
	scheme := runtime.NewScheme()
	_ = experimentcontrollercomv1alpha1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	sourceDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "source-deployment",
			Namespace: "test-namespace",
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
			Name:       "experiment-cr",
			Namespace:  "test-namespace",
			Finalizers: []string{experimentDeploymentFinalizer},
		},
		Spec: experimentcontrollercomv1alpha1.ExperimentDeploymentSpec{
			SourceRef: experimentcontrollercomv1alpha1.SourceRef{
				Kind: experimentcontrollercomv1alpha1.SourceKindDeployment,
				Name: "source-deployment",
			},
			Replicas:     func() *int32 { r := int32(1); return &r }(),
			OverrideSpec: apiextensionsv1.JSON{Raw: []byte("{}")},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sourceDeployment, experimentCR).Build()
		reconciler := &ExperimentDeploymentReconciler{
			Client:   fakeClient,
			Scheme:   scheme,
			Recorder: record.NewFakeRecorder(100),
		}
		ctx := context.Background()
		b.StartTimer()

		_, err := reconciler.reconcileDeploymentExperiment(ctx, experimentCR, "test-namespace")
		if err != nil {
			b.Fatal(err)
		}
	}
}
