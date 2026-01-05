package main

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/utils/ptr"

	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdispec "tags.cncf.io/container-device-interface/specs-go"
)

const (
	defaultHostDeviceTarget = "/dev/null"
	defaultDeviceMode       = 0o660
	nullDeviceMajor         = 1
	nullDeviceMinor         = 3
)

type computeDomainNvcdiDevice struct {
	deviceRoot string
}

func newComputeDomainNvcdiDevice(root string) (*computeDomainNvcdiDevice, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create device root: %w", err)
	}
	return &computeDomainNvcdiDevice{
		deviceRoot: root,
	}, nil
}

func (d *computeDomainNvcdiDevice) ContainerEdits(info *DomainInfo) (*cdiapi.ContainerEdits, error) {
	nodes, err := d.ensureDomainDeviceNodes(info)
	if err != nil {
		return nil, err
	}

	return &cdiapi.ContainerEdits{
		ContainerEdits: &cdispec.ContainerEdits{
			DeviceNodes: nodes,
		},
	}, nil
}

func (d *computeDomainNvcdiDevice) ensureDomainDeviceNodes(info *DomainInfo) ([]*cdispec.DeviceNode, error) {
	domainDir := filepath.Join(d.deviceRoot, info.DomainID)
	if err := os.MkdirAll(domainDir, 0o750); err != nil {
		return nil, fmt.Errorf("prepare domain device directory: %w", err)
	}

	channelPath := filepath.Join(domainDir, "channel-0")
	if err := ensureSymlink(channelPath, defaultHostDeviceTarget); err != nil {
		return nil, fmt.Errorf("ensure channel device: %w", err)
	}

	return []*cdispec.DeviceNode{
		{
			Path:     channelPath,
			Type:     "c",
			FileMode: ptr.To(os.FileMode(defaultDeviceMode)),
			Major:    nullDeviceMajor,
			Minor:    nullDeviceMinor,
		},
	}, nil
}

func ensureSymlink(path, target string) error {
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			current, readErr := os.Readlink(path)
			if readErr == nil && current == target {
				return nil
			}
			if err := os.Remove(path); err != nil {
				return err
			}
		} else {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(target, path)
}
