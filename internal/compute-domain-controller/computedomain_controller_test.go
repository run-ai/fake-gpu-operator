/*
 * Copyright 2025 The Kubernetes Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package computedomaincontroller_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	controller "github.com/run-ai/fake-gpu-operator/internal/compute-domain-controller"

	computedomainv1beta1 "github.com/NVIDIA/k8s-dra-driver-gpu/api/nvidia.com/resource/v1beta1"
	"github.com/run-ai/fake-gpu-operator/pkg/compute-domain/consts"
)

func TestComputeDomainReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = resourceapi.AddToScheme(scheme)
	_ = computedomainv1beta1.AddToScheme(scheme)

	tests := map[string]struct {
		computeDomain            *computedomainv1beta1.ComputeDomain
		existingObjects          []client.Object
		expectedWorkloadTemplate bool
		expectedFinalizer        bool
	}{
		"new ComputeDomain creates templates": {
			computeDomain: &computedomainv1beta1.ComputeDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-domain",
					Namespace: "default",
					UID:       "test-uid",
				},
			},
			expectedWorkloadTemplate: true,
			expectedFinalizer:        true,
		},
		"deleted ComputeDomain removes templates": {
			computeDomain: &computedomainv1beta1.ComputeDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-domain",
					Namespace:         "default",
					UID:               "test-uid",
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Finalizers:        []string{consts.ComputeDomainFinalizer},
				},
			},
			existingObjects: []client.Object{
				&resourceapi.ResourceClaimTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-domain",
						Namespace: "default",
						Labels: map[string]string{
							"resource.nvidia.com/computeDomain": "test-domain",
						},
						Finalizers: []string{consts.ComputeDomainFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "ComputeDomain",
								Name: "test-domain",
							},
						},
					},
				},
			},
			expectedWorkloadTemplate: false,
			expectedFinalizer:        false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup
			objs := []client.Object{test.computeDomain}
			if len(test.existingObjects) > 0 {
				objs = append(objs, test.existingObjects...)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objs...).
				Build()

			reconciler := &controller.ComputeDomainReconciler{
				Client: fakeClient,
				Scheme: scheme,
			}

			// Execute
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      test.computeDomain.GetName(),
					Namespace: test.computeDomain.GetNamespace(),
				},
			}

			result, err := reconciler.Reconcile(context.Background(), req)

			// Verify
			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, result)

			workloadTemplate := &resourceapi.ResourceClaimTemplate{}
			err = fakeClient.Get(context.Background(), types.NamespacedName{
				Name:      "test-domain",
				Namespace: "default",
			}, workloadTemplate)
			// Check ResourceClaimTemplates
			if test.expectedWorkloadTemplate {
				assert.NoError(t, err)
				// Check config section
				assert.Len(t, workloadTemplate.Spec.Spec.Devices.Config, 1)
				assert.Equal(t, consts.ComputeDomainDriverName, workloadTemplate.Spec.Spec.Devices.Config[0].Opaque.Driver)
				// Check requests - only channel request expected
				assert.Len(t, workloadTemplate.Spec.Spec.Devices.Requests, 1)
				assert.Equal(t, "channel", workloadTemplate.Spec.Spec.Devices.Requests[0].Name)
				assert.Equal(t, resourceapi.DeviceAllocationModeExactCount, workloadTemplate.Spec.Spec.Devices.Requests[0].Exactly.AllocationMode)
				assert.Equal(t, int64(1), workloadTemplate.Spec.Spec.Devices.Requests[0].Exactly.Count)
				assert.Equal(t, consts.ComputeDomainWorkloadDeviceClass, workloadTemplate.Spec.Spec.Devices.Requests[0].Exactly.DeviceClassName)
				// Check labels
				assert.Equal(t, test.computeDomain.GetName(), workloadTemplate.Labels["resource.nvidia.com/computeDomain"])
				assert.Equal(t, "workload", workloadTemplate.Labels["resource.nvidia.com/computeDomainTarget"])
				// Check labels copied into generated claims
				assert.Equal(t, test.computeDomain.GetName(), workloadTemplate.Spec.Labels["nvidia.com/computeDomain"])
				// Check finalizers
				assert.Contains(t, workloadTemplate.Finalizers, consts.ComputeDomainFinalizer)
			} else {
				assert.Error(t, err)
				assert.NoError(t, client.IgnoreNotFound(err))
			}

			if !test.computeDomain.DeletionTimestamp.IsZero() {
				return
			}

			// Check finalizer
			updatedDomain := &computedomainv1beta1.ComputeDomain{}
			err = fakeClient.Get(context.Background(), req.NamespacedName, updatedDomain)
			require.NoError(t, err)

			finalizers := updatedDomain.GetFinalizers()
			hasFinalizer := false
			for _, f := range finalizers {
				if f == consts.ComputeDomainFinalizer {
					hasFinalizer = true
					break
				}
			}
			assert.Equal(t, test.expectedFinalizer, hasFinalizer)
		})
	}
}

func TestComputeDomainReconciler_Reconcile_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = resourceapi.AddToScheme(scheme)
	_ = computedomainv1beta1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	reconciler := &controller.ComputeDomainReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}
