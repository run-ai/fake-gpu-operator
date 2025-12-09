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

import "github.com/spf13/pflag"

// Options contains the tunable settings for the controller binary.
type Options struct {
	MetricsBindAddress string
	HealthProbeAddress string
	LeaderElection     bool
}

// NewOptions returns Options populated with default values.
func NewOptions() *Options {
	return &Options{
		MetricsBindAddress: ":8080",
		HealthProbeAddress: ":8081",
	}
}

// AddFlags adds CLI flags for the controller binary.
func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.MetricsBindAddress, "metrics-bind-address", o.MetricsBindAddress,
		"The address the metric endpoint binds to.")
	fs.StringVar(&o.HealthProbeAddress, "health-probe-bind-address", o.HealthProbeAddress,
		"The address the health probe endpoint binds to.")
	fs.BoolVar(&o.LeaderElection, "leader-elect", o.LeaderElection,
		"Enable leader election for the controller manager.")
}


