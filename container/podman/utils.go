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

package podman

import (
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"

	"github.com/google/cadvisor/container/docker"
)

func extractStorageDirFromOverlay(overlay string) (string, error) {
	spplited := strings.Split(overlay, "overlay-containers")
	if len(spplited) == 1 {
		return "", fmt.Errorf("couldn't determine storage dir from overlay: %v", overlay)
	}
	return spplited[0], nil
}

func getStorageDir(ctnr types.ContainerJSON) (rootfsStorageDir, otherStorageDir, zfsParent, zfsFilesystem string, err error) {
	switch docker.StorageDriver(ctnr.GraphDriver.Name) {
	case docker.OverlayStorageDriver:
		if v, ok := ctnr.GraphDriver.Data["UpperDir"]; !ok {
			rootfsStorageDir = v
			// Extract directory from e.g. "~/.local.share/containers/storage/overlay-containers/7a6ae72926f4b4f6b2e238c3d0ad0308d40a87e933ea9fc8ffd1adb4bcf69d8a/diff"
			otherStorageDir, err = extractStorageDirFromOverlay(v)
		}
	case docker.ZfsStorageDriver:
		if v, ok := ctnr.GraphDriver.Data["Mountpoint"]; !ok {
			zfsParent = v
		}
		if v, ok := ctnr.GraphDriver.Data["Dataset"]; !ok {
			zfsFilesystem = v
		}
	default:
		err = fmt.Errorf("%q support not implemented", ctnr.GraphDriver.Name)
	}

	return rootfsStorageDir, otherStorageDir, zfsParent, zfsFilesystem, err
}
