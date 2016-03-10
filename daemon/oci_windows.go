package daemon

import (
	"fmt"
	"strings"

	"github.com/docker/docker/container"
	"github.com/docker/docker/layer"
	"github.com/docker/docker/libcontainerd"
	"github.com/docker/docker/libcontainerd/windowsoci"
	"github.com/docker/docker/oci"
)

func (daemon *Daemon) createSpec(c *container.Container) (*libcontainerd.Spec, error) {
	s := oci.DefaultSpec()

	linkedEnv, err := daemon.setupLinkedContainers(c)
	if err != nil {
		return nil, err
	}

	rootUID, rootGID := daemon.GetRemappedUIDGID()
	if err := c.SetupWorkingDirectory(rootUID, rootGID); err != nil {
		return nil, err
	}

	img, err := daemon.imageStore.Get(c.ImageID)
	if err != nil {
		return nil, fmt.Errorf("Failed to graph.Get on ImageID %s - %s", c.ImageID, err)
	}

	// In base spec
	s.Hostname = c.FullHostname()

	// In s.Mounts
	mounts, err := daemon.setupMounts(c)
	if err != nil {
		return nil, err
	}
	for _, mount := range mounts {
		s.Mounts = append(s.Mounts, windowsoci.Mount{
			Source:      mount.Source,
			Destination: mount.Destination,
			Readonly:    !mount.Writable,
		})
	}

	// In s.Process
	s.Process.Args = append([]string{c.Path}, c.Args...)
	s.Process.Cwd = c.Config.WorkingDir
	s.Process.Env = c.CreateDaemonEnvironment(linkedEnv)
	s.Process.InitialConsoleSize = c.HostConfig.ConsoleSize
	s.Process.Terminal = c.Config.Tty
	s.Process.User.User = c.Config.User

	// In spec.Root
	s.Root.Path = c.BaseFS
	s.Root.Readonly = c.HostConfig.ReadonlyRootfs

	// In s.Windows
	s.Windows.FirstStart = !c.HasBeenStartedBefore

	// s.Windows.LayerFolder
	m, err := c.RWLayer.Metadata()
	if err != nil {
		return nil, fmt.Errorf("Failed to get layer metadata - %s", err)
	}
	s.Windows.LayerFolder = m["dir"]

	// s.Windows.LayerPaths
	var layerPaths []string
	if img.RootFS != nil && img.RootFS.Type == "layers+base" {
		max := len(img.RootFS.DiffIDs)
		for i := 0; i <= max; i++ {
			img.RootFS.DiffIDs = img.RootFS.DiffIDs[:i]
			path, err := layer.GetLayerPath(daemon.layerStore, img.RootFS.ChainID())
			if err != nil {
				return nil, fmt.Errorf("Failed to get layer path from graphdriver %s for ImageID %s - %s", daemon.layerStore, img.RootFS.ChainID(), err)
			}
			// Reverse order, expecting parent most first
			layerPaths = append([]string{path}, layerPaths...)
		}
	}
	s.Windows.LayerPaths = layerPaths

	// In s.Windows.Networking (TP4 back compat)
	parts := strings.SplitN(string(c.HostConfig.NetworkMode), ":", 2)
	switch parts[0] {
	case "none":
	case "default", "": // empty string to support existing containers
		if !c.Config.NetworkDisabled {
			s.Windows.Networking = &windowsoci.Networking{
				MacAddress:   c.Config.MacAddress,
				Bridge:       daemon.configStore.bridgeConfig.VirtualSwitchName,
				PortBindings: c.HostConfig.PortBindings,
			}
		}
	default:
		return nil, fmt.Errorf("invalid network mode: %s", c.HostConfig.NetworkMode)
	}

	// In s.Windows.Networking
	// @darrenstahlmsft Fix this when this PR is rebased on master 3/11
	//	s.Windows.Network = &windowsoci.Networking{
	//		EndpointList: ???? something????,
	//	}

	// In s.Windows.Resources
	// @darrenstahlmsft implement these resources
	cpuShares := uint64(c.HostConfig.CPUShares)
	s.Windows.Resources = &windowsoci.Resources{
		CPU: &windowsoci.CPU{
			//TODO Count: ...,
			//TODO Percent: ...,
			Shares: &cpuShares,
		},
		Memory: &windowsoci.Memory{
		//TODO Limit: ...,
		//TODO Reservation: ...,
		},
		Network: &windowsoci.Network{
		//TODO Bandwidth: ...,
		},
		Storage: &windowsoci.Storage{
		//TODO Bps: ...,
		//TODO Iops: ...,
		//TODO SandboxSize: ...,
		},
	}

	// BUGBUG - Next problem. This was an exec opt. Where do we now get these?
	// Come back to this when add Xenon support.
	//	var hvPartition bool
	//	// Work out the isolation (whether it is a hypervisor partition)
	//	if c.HostConfig.Isolation.IsDefault() {
	//		// Not specified by caller. Take daemon default
	//		hvPartition = windows.DefaultIsolation.IsHyperV()
	//	} else {
	//		// Take value specified by caller
	//		hvPartition = c.HostConfig.Isolation.IsHyperV()
	//	}

	//		Isolation:   string(c.HostConfig.Isolation),
	//		HvPartition: hvPartition,
	//	}

	return (*libcontainerd.Spec)(&s), nil
}
