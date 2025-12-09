/*
 * Copyright 2023 The Kubernetes Authors.
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

package dra_plugin_gpu

import (
	"path/filepath"

	coreclientset "k8s.io/client-go/kubernetes"
)

const (
	DriverPluginCheckpointFile = "checkpoint.json"
	DriverName                 = "gpu.nvidia.com" // Override driver name for deviceclass compatibility
)

// Flags contains configuration flags for the DRA plugin
type Flags struct {
	NodeName                      string
	CDIRoot                       string
	KubeletRegistrarDirectoryPath string
	KubeletPluginsDirectoryPath   string
	HealthcheckPort               int
}

// Config contains the configuration for the DRA plugin
type Config struct {
	Flags         *Flags
	CoreClient    coreclientset.Interface
	CancelMainCtx func(error)
}

// DriverPluginPath returns the path to the driver plugin directory
func (c *Config) DriverPluginPath() string {
	return filepath.Join(c.Flags.KubeletPluginsDirectoryPath, DriverName)
}

