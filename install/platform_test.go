package install

import (
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestComposeAsset(t *testing.T) {
	cases := []struct {
		p    Platform
		want string
	}{
		{Platform{"linux", "amd64"}, "docker-compose-linux-x86_64"},
		{Platform{"linux", "arm64"}, "docker-compose-linux-aarch64"},
		{Platform{"darwin", "arm64"}, "docker-compose-darwin-aarch64"},
		{Platform{"windows", "amd64"}, "docker-compose-windows-x86_64.exe"},
	}
	for _, tc := range cases {
		t.Run(tc.p.OS+"/"+tc.p.Arch, func(t *testing.T) {
			got, err := tc.p.composeAsset()
			assert.NilError(t, err)
			assert.Check(t, cmp.Equal(got, tc.want))
		})
	}
}

func TestUnsupportedPlatform(t *testing.T) {
	_, err := Platform{"freebsd", "amd64"}.composeAsset()
	assert.Check(t, cmp.ErrorContains(err, "unsupported platform freebsd/amd64"))
}
