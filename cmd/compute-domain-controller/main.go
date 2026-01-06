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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/spf13/pflag"
	"go.uber.org/zap/zapcore"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	controller "github.com/run-ai/fake-gpu-operator/internal/compute-domain-controller"

	computedomainv1beta1 "github.com/NVIDIA/k8s-dra-driver-gpu/api/nvidia.com/resource/v1beta1"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = resourceapi.AddToScheme(scheme)
	_ = computedomainv1beta1.AddToScheme(scheme)
}

func main() {
	opts := NewOptions()

	fs := pflag.NewFlagSet("fake-compute-domain-controller", pflag.ExitOnError)
	opts.AddFlags(fs)

	zapOpts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	zapOpts.BindFlags(flag.CommandLine)

	fs.AddGoFlagSet(flag.CommandLine)

	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		os.Exit(1)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))
	ctx := ctrl.SetupSignalHandler()

	if err := run(ctx, opts); err != nil {
		setupLog.Error(err, "controller exited with error")
		os.Exit(1)
	}
}

func run(ctx context.Context, options *Options) error {
	cfg := ctrl.GetConfigOrDie()

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: options.HealthProbeAddress,
		LeaderElection:         options.LeaderElection,
		LeaderElectionID:       "fake-compute-domain-controller",
	})
	if err != nil {
		return fmt.Errorf("failed to create controller manager: %w", err)
	}

	reconciler := &controller.ComputeDomainReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to setup reconciler: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to add health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to add ready check: %w", err)
	}

	setupLog.Info("starting manager")
	return mgr.Start(ctx)
}
