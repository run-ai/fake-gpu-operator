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

package computedomaincontroller

import (
	"context"
	"fmt"

	resourceapi "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	computedomainv1beta1 "github.com/NVIDIA/k8s-dra-driver-gpu/api/nvidia.com/resource/v1beta1"
	"github.com/run-ai/fake-gpu-operator/pkg/compute-domain/consts"
)

const (
	// DefaultComputeDomainAllocationMode is the default allocation mode when not specified
	DefaultComputeDomainAllocationMode = "Single"
)

// ComputeDomainReconciler watches ComputeDomain resources and keeps the
// associated ResourceClaimTemplates in sync.
type ComputeDomainReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=resource.nvidia.com,resources=computedomains,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=resource.nvidia.com,resources=computedomains/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=resource.nvidia.com,resources=computedomains/finalizers,verbs=update
//+kubebuilder:rbac:groups=resource.k8s.io,resources=resourceclaimtemplates,verbs=get;list;watch;create;update;patch;delete

func (r *ComputeDomainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	domain := &computedomainv1beta1.ComputeDomain{}
	if err := r.Get(ctx, req.NamespacedName, domain); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if domain.DeletionTimestamp.IsZero() {
		if err := r.ensureFinalizer(ctx, domain); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.ensureResourceClaimTemplates(ctx, domain); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		if err := r.handleDeletion(ctx, domain); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	logger.V(4).Info("reconciled ComputeDomain", "namespace", domain.Namespace, "name", domain.Name)
	return ctrl.Result{}, nil
}

func (r *ComputeDomainReconciler) ensureFinalizer(ctx context.Context, domain *computedomainv1beta1.ComputeDomain) error {
	if controllerutil.ContainsFinalizer(domain, consts.ComputeDomainFinalizer) {
		return nil
	}

	controllerutil.AddFinalizer(domain, consts.ComputeDomainFinalizer)
	return r.Update(ctx, domain)
}

func (r *ComputeDomainReconciler) handleDeletion(ctx context.Context, domain *computedomainv1beta1.ComputeDomain) error {
	if !controllerutil.ContainsFinalizer(domain, consts.ComputeDomainFinalizer) {
		return nil
	}

	if err := r.deleteResourceClaimTemplates(ctx, domain); err != nil {
		return err
	}

	controllerutil.RemoveFinalizer(domain, consts.ComputeDomainFinalizer)
	return r.Update(ctx, domain)
}

func (r *ComputeDomainReconciler) ensureResourceClaimTemplates(ctx context.Context, domain *computedomainv1beta1.ComputeDomain) error {
	return r.ensureTemplate(ctx, domain, domain.Name, consts.ComputeDomainWorkloadDeviceClass, "workload")
}

func (r *ComputeDomainReconciler) getAllocationMode(domain *computedomainv1beta1.ComputeDomain) string {
	if domain.Spec.Channel != nil && domain.Spec.Channel.AllocationMode != "" {
		return domain.Spec.Channel.AllocationMode
	}
	return DefaultComputeDomainAllocationMode
}

func (r *ComputeDomainReconciler) ensureTemplate(
	ctx context.Context,
	domain *computedomainv1beta1.ComputeDomain,
	name string,
	deviceClass string,
	templateType string,
) error {
	key := client.ObjectKey{Namespace: domain.Namespace, Name: name}
	existing := &resourceapi.ResourceClaimTemplate{}
	err := r.Get(ctx, key, existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	template := &resourceapi.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: domain.Namespace,
			Labels: map[string]string{
				"resource.nvidia.com/computeDomain":       domain.Name,
				"resource.nvidia.com/computeDomainTarget": templateType,
			},
			Finalizers: []string{
				"resource.nvidia.com/computeDomain",
			},
		},
		Spec: resourceapi.ResourceClaimTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"nvidia.com/computeDomain": domain.Name,
				},
			},
			Spec: resourceapi.ResourceClaimSpec{
				Devices: resourceapi.DeviceClaim{
					Config: []resourceapi.DeviceClaimConfiguration{
						{
							DeviceConfiguration: resourceapi.DeviceConfiguration{
								Opaque: &resourceapi.OpaqueDeviceConfiguration{
									Driver: consts.ComputeDomainDriverName,
									Parameters: runtime.RawExtension{
										Raw: []byte(fmt.Sprintf(`{
											"allocationMode": "%s",
											"apiVersion": "resource.nvidia.com/v1beta1",
											"domainID": "%s",
											"kind": "ComputeDomainChannelConfig"
										}`, r.getAllocationMode(domain), domain.UID)),
									},
								},
							},
						},
					},
					Requests: []resourceapi.DeviceRequest{
						{
							Name: "channel",
							Exactly: &resourceapi.ExactDeviceRequest{
								AllocationMode:  resourceapi.DeviceAllocationModeExactCount,
								Count:           1,
								DeviceClassName: deviceClass,
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(domain, template, r.Scheme); err != nil {
		return err
	}

	return client.IgnoreAlreadyExists(r.Create(ctx, template))
}

func (r *ComputeDomainReconciler) deleteResourceClaimTemplates(ctx context.Context, domain *computedomainv1beta1.ComputeDomain) error {
	template := &resourceapi.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      domain.Name,
			Namespace: domain.Namespace,
		},
	}
	if err := r.Delete(ctx, template); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// SetupWithManager wires the reconciler into the controller-runtime manager.
func (r *ComputeDomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&computedomainv1beta1.ComputeDomain{}).
		Owns(&resourceapi.ResourceClaimTemplate{}).
		Complete(r)
}
