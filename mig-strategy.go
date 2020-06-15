/*
 * Copyright (c) 2020, NVIDIA CORPORATION.  All rights reserved.
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
	"fmt"

	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	MigStrategyDisabled = "disabled"
	MigStrategyNone     = "none"
	MigStrategySingle   = "single"
)

type MigStrategyResourceSet map[string]struct{}

type MigStrategy interface {
	GetPlugins() []*NvidiaDevicePlugin
	MatchesResource(mig *nvml.Device, resource string) bool
}

func NewMigStrategy(strategy string) (MigStrategy, error) {
	switch strategy {
	case MigStrategyDisabled:
		return &migStrategyDisabled{}, nil
	case MigStrategyNone:
		return &migStrategyNone{}, nil
	case MigStrategySingle:
		return &migStrategySingle{}, nil
	}
	return nil, fmt.Errorf("Unknown strategy: %v", strategy)
}

type migStrategyDisabled struct{}
type migStrategyNone struct{}
type migStrategySingle struct{}

// getAllMigDevices() across all full GPUs
func getAllMigDevices() []*nvml.Device {
	n, err := nvml.GetDeviceCount()
	check(err)

	var migs []*nvml.Device
	for i := uint(0); i < n; i++ {
		d, err := nvml.NewDeviceLite(i)
		check(err)

		migEnabled, err := d.IsMigEnabled()
		check(err)

		if !migEnabled {
			continue
		}

		devs, err := d.GetMigDevices()
		check(err)

		migs = append(migs, devs...)
	}

	return migs
}

// migStrategyDisabled
func (s *migStrategyDisabled) GetPlugins() []*NvidiaDevicePlugin {
	return []*NvidiaDevicePlugin{
		NewNvidiaDevicePlugin(
			"nvidia.com/gpu",
			NewGpuDeviceManager(false), // Enumerate device even if MIG enabled
			"NVIDIA_VISIBLE_DEVICES",
			pluginapi.DevicePluginPath+"nvidia-gpu.sock"),
	}
}

func (s *migStrategyDisabled) MatchesResource(mig *nvml.Device, resource string) bool {
	panic("Should never be called")
	return false
}

// migStrategyNone
func (s *migStrategyNone) GetPlugins() []*NvidiaDevicePlugin {
	return []*NvidiaDevicePlugin{
		NewNvidiaDevicePlugin(
			"nvidia.com/gpu",
			NewGpuDeviceManager(true), // Skip device if MIG enabled
			"NVIDIA_VISIBLE_DEVICES",
			pluginapi.DevicePluginPath+"nvidia-gpu.sock"),
	}
}

func (s *migStrategyNone) MatchesResource(mig *nvml.Device, resource string) bool {
	panic("Should never be called")
	return false
}

// migStrategySingle
func (s *migStrategySingle) GetPlugins() []*NvidiaDevicePlugin {
	resources := make(MigStrategyResourceSet)
	for _, mig := range getAllMigDevices() {
		r := s.getResourceName(mig)
		resources[r] = struct{}{}
	}

	if len(resources) != 1 {
		panic("More than one MIG device type present on node")
	}

	return []*NvidiaDevicePlugin{
		NewNvidiaDevicePlugin(
			"nvidia.com/gpu",
			NewMigDeviceManager(s, "gpu"),
			"NVIDIA_VISIBLE_DEVICES",
			pluginapi.DevicePluginPath+"nvidia-gpu.sock"),
	}
}

func (s *migStrategySingle) getResourceName(mig *nvml.Device) string {
	attr, err := mig.GetAttributes()
	check(err)

	g := attr.GpuInstanceSliceCount
	c := attr.ComputeInstanceSliceCount
	gb := ((attr.MemorySizeMB + 1000 - 1) / 1000)
	r := fmt.Sprintf("mig-%dc.%dg.%dgb", c, g, gb)

	return r
}

func (s *migStrategySingle) MatchesResource(mig *nvml.Device, resource string) bool {
	return true
}
