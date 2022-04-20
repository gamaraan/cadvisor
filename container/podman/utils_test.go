package podman

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractStorageDirFromOverlay(t *testing.T) {
	for _, tc := range []struct {
		overlay  string
		expected string
		err      error
	}{
		{
			"~/.local.share/containers",
			"",
			errors.New("couldn't determine storage dir from overlay: ~/.local.share/containers"),
		},
		{
			"~/.local.share/containers/storage/overlay-containers/7a6ae72926f4b4f6b2e238c3d0ad0308d40a87e933ea9fc8ffd1adb4bcf69d8a/diff",
			"~/.local.share/containers/storage/",
			nil,
		},
	} {
		got, err := extractStorageDirFromOverlay(tc.overlay)
		assert.Equal(t, tc.expected, got)
		if tc.err != nil {
			assert.Equal(t, tc.err, err)
		} else {
			assert.NoError(t, err)
		}
	}
}
