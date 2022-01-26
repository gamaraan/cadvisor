// Copyright 2022 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package docker

import (
	"fmt"

	"k8s.io/klog/v2"

	"github.com/google/cadvisor/container"
	"github.com/google/cadvisor/container/common"
	"github.com/google/cadvisor/devicemapper"
	"github.com/google/cadvisor/fs"
	info "github.com/google/cadvisor/info/v1"
	"github.com/google/cadvisor/zfs"
)

func GetFsStats(
	stats *info.ContainerStats,
	machineInfoFactory info.MachineInfoFactory,
	metrics container.MetricSet,
	storageDriver StorageDriver,
	fsHandler common.FsHandler,
	globalFsInfo fs.FsInfo,
	poolName string,
	rootfsStorageDir string,
	zfsParent string,
) error {
	mi, err := machineInfoFactory.GetMachineInfo()
	if err != nil {
		return err
	}

	if metrics.Has(container.DiskIOMetrics) {
		common.AssignDeviceNamesToDiskStats((*common.MachineInfoNamer)(mi), &stats.DiskIo)
	}

	if metrics.Has(container.DiskUsageMetrics) {
		var device string
		switch storageDriver {
		case DevicemapperStorageDriver:
			device = poolName
		case AufsStorageDriver, OverlayStorageDriver, Overlay2StorageDriver, VfsStorageDriver:
			deviceInfo, err := globalFsInfo.GetDirFsDevice(rootfsStorageDir)
			if err != nil {
				return fmt.Errorf("unable to determine device info for dir: %v: %v", rootfsStorageDir, err)
			}
			device = deviceInfo.Device
		case ZfsStorageDriver:
			device = zfsParent
		default:
			return nil
		}

		var (
			limit  uint64
			fsType string
		)

		var fsInfo *info.FsInfo

		for _, fs := range mi.Filesystems {
			if fs.Device == device {
				limit = fs.Capacity
				fsType = fs.Type
				fsInfo = &fs
				break
			}
		}

		fsStat := info.FsStats{Device: device, Type: fsType, Limit: limit}
		usage := fsHandler.Usage()
		fsStat.BaseUsage = usage.BaseUsageBytes
		fsStat.Usage = usage.TotalUsageBytes
		fsStat.Inodes = usage.InodeUsage

		if fsInfo != nil {
			fileSystems, err := globalFsInfo.GetGlobalFsInfo()

			if err == nil {
				addDiskStats(fileSystems, fsInfo, &fsStat)
			} else {
				klog.Errorf("Unable to obtain diskstats for filesystem %s: %v", fsStat.Device, err)
			}
		}

		stats.Filesystem = append(stats.Filesystem, fsStat)
	}

	return nil
}

func addDiskStats(fileSystems []fs.Fs, fsInfo *info.FsInfo, fsStats *info.FsStats) {
	if fsInfo == nil {
		return
	}

	for _, fileSys := range fileSystems {
		if fsInfo.DeviceMajor == fileSys.DiskStats.Major &&
			fsInfo.DeviceMinor == fileSys.DiskStats.Minor {
			fsStats.ReadsCompleted = fileSys.DiskStats.ReadsCompleted
			fsStats.ReadsMerged = fileSys.DiskStats.ReadsMerged
			fsStats.SectorsRead = fileSys.DiskStats.SectorsRead
			fsStats.ReadTime = fileSys.DiskStats.ReadTime
			fsStats.WritesCompleted = fileSys.DiskStats.WritesCompleted
			fsStats.WritesMerged = fileSys.DiskStats.WritesMerged
			fsStats.SectorsWritten = fileSys.DiskStats.SectorsWritten
			fsStats.WriteTime = fileSys.DiskStats.WriteTime
			fsStats.IoInProgress = fileSys.DiskStats.IoInProgress
			fsStats.IoTime = fileSys.DiskStats.IoTime
			fsStats.WeightedIoTime = fileSys.DiskStats.WeightedIoTime
			break
		}
	}
}

// dockerFsHandler is a composite FsHandler implementation the incorporates
// the common fs handler, a devicemapper ThinPoolWatcher, and a zfsWatcher
type dockerFsHandler struct {
	fsHandler common.FsHandler

	// thinPoolWatcher is the devicemapper thin pool watcher
	thinPoolWatcher *devicemapper.ThinPoolWatcher
	// deviceID is the id of the container's fs device
	deviceID string

	// zfsWatcher is the zfs filesystem watcher
	zfsWatcher *zfs.ZfsWatcher
	// zfsFilesystem is the docker zfs filesystem
	zfsFilesystem string
}

var _ common.FsHandler = &dockerFsHandler{}

func (h *dockerFsHandler) Start() {
	h.fsHandler.Start()
}

func (h *dockerFsHandler) Stop() {
	h.fsHandler.Stop()
}

func (h *dockerFsHandler) Usage() common.FsUsage {
	usage := h.fsHandler.Usage()

	// When devicemapper is the storage driver, the base usage of the container comes from the thin pool.
	// We still need the result of the fsHandler for any extra storage associated with the container.
	// To correctly factor in the thin pool usage, we should:
	// * Usage the thin pool usage as the base usage
	// * Calculate the overall usage by adding the overall usage from the fs handler to the thin pool usage
	if h.thinPoolWatcher != nil {
		thinPoolUsage, err := h.thinPoolWatcher.GetUsage(h.deviceID)
		if err != nil {
			// TODO: ideally we should keep track of how many times we failed to get the usage for this
			// device vs how many refreshes of the cache there have been, and display an error e.g. if we've
			// had at least 1 refresh and we still can't find the device.
			klog.V(5).Infof("unable to get fs usage from thin pool for device %s: %v", h.deviceID, err)
		} else {
			usage.BaseUsageBytes = thinPoolUsage
			usage.TotalUsageBytes += thinPoolUsage
		}
	}

	if h.zfsWatcher != nil {
		zfsUsage, err := h.zfsWatcher.GetUsage(h.zfsFilesystem)
		if err != nil {
			klog.V(5).Infof("unable to get fs usage from zfs for filesystem %s: %v", h.zfsFilesystem, err)
		} else {
			usage.BaseUsageBytes = zfsUsage
			usage.TotalUsageBytes += zfsUsage
		}
	}
	return usage
}
