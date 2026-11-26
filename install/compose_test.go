package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func fakeComposeBinary(version string) []byte {
	return []byte(fmt.Sprintf("#!/bin/sh\necho 'Docker Compose version %s'\n", version))
}

func makeComposeServer(t *testing.T, version, asset string, binary []byte) *httptest.Server {
	t.Helper()
	h := sha256.Sum256(binary)
	checksum := hex.EncodeToString(h[:])
	sumsContent := fmt.Sprintf("%s  %s\n", checksum, asset)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docker/compose/releases/download/" + version + "/checksums.txt":
			fmt.Fprint(w, sumsContent)
		case "/docker/compose/releases/download/" + version + "/" + asset:
			_, _ = w.Write(binary)
		case "/docker/compose/releases/latest":
			http.Redirect(w, r, "/docker/compose/releases/tag/"+version, http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestComposeInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	const version = "v2.27.2"
	p := Platform{"linux", "amd64"}
	asset, err := p.composeAsset()
	assert.NilError(t, err)

	binary := fakeComposeBinary(version)
	srv := makeComposeServer(t, version, asset, binary)
	defer srv.Close()

	pluginDir := t.TempDir()
	installDir := t.TempDir()
	// Use only installDir on PATH — fake scripts use absolute #!/bin/sh shebangs,
	// and we must not let a real "docker" on the executor's PATH cause compose to
	// report an installed version and short-circuit the download.
	t.Setenv("PATH", installDir)

	ins := testInstaller(srv)
	ins.InstallDir = installDir
	ins.PluginDir = pluginDir

	ctx := context.Background()
	res, err := ins.Compose(ctx, p, version)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Tool, "compose"))
	assert.Check(t, cmp.Equal(res.Version, version))

	_, err = os.Stat(filepath.Join(pluginDir, "docker-compose"))
	assert.NilError(t, err)
}

func TestComposeChecksumMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	const version = "v2.27.2"
	p := Platform{"linux", "amd64"}
	asset, err := p.composeAsset()
	assert.NilError(t, err)

	binary := fakeComposeBinary(version)
	badSums := fmt.Sprintf("0000000000000000000000000000000000000000000000000000000000000000  %s\n", asset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docker/compose/releases/download/" + version + "/checksums.txt":
			fmt.Fprint(w, badSums)
		case "/docker/compose/releases/download/" + version + "/" + asset:
			_, _ = w.Write(binary)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	pluginDir := t.TempDir()
	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.PluginDir = pluginDir
	// Isolate PATH so no real "docker" on the executor reports an installed
	// compose version and causes a keep-existing short-circuit.
	t.Setenv("PATH", t.TempDir())

	ctx := context.Background()
	_, err = ins.Compose(ctx, p, version)
	assert.Check(t, cmp.ErrorContains(err, "SHA-256 mismatch"))
}

// TestComposeKeepsAnInstalledVersion checks that a Compose already installed
// is used as it is, with nothing downloaded and nothing linked.
func TestComposeKeepsAnInstalledVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	fakeDockerDir := t.TempDir()
	dockerScript := `#!/bin/sh
case "$1" in
  compose) case "$2" in version) echo "Docker Compose version v2.27.2" ;; esac ;;
esac
`
	assert.NilError(t, os.WriteFile(filepath.Join(fakeDockerDir, "docker"), []byte(dockerScript), 0o755))
	t.Setenv("PATH", fakeDockerDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected download of %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.PluginDir = t.TempDir()

	for _, want := range []string{"", "v2.27.2"} {
		res, err := ins.Compose(context.Background(), Platform{"linux", "amd64"}, want)
		assert.NilError(t, err)
		assert.Check(t, cmp.Equal(res.Kept, true), "want %q", want)
		assert.Check(t, cmp.Equal(strings.TrimPrefix(res.Version, "v"), "2.27.2"))
		assert.Check(t, cmp.Equal(res.Dir, ""))
	}
}

// TestComposeReplacesAVersionTooOld installs the pinned Compose over one that
// lacks flags setup-compose uses, as cimg/base:stable ships.
func TestComposeReplacesAVersionTooOld(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}
	p := Platform{"linux", "amd64"}
	asset, err := p.composeAsset()
	assert.NilError(t, err)
	srv := makeComposeServer(t, DefaultComposeVersion, asset, fakeComposeBinary(DefaultComposeVersion))
	defer srv.Close()

	fakeDockerDir := t.TempDir()
	dockerScript := `#!/bin/sh
case "$1 $2" in "compose version") echo "Docker Compose version v2.6.0" ;; esac
`
	assert.NilError(t, os.WriteFile(filepath.Join(fakeDockerDir, "docker"), []byte(dockerScript), 0o755))
	installDir := t.TempDir()
	t.Setenv("PATH", installDir+string(os.PathListSeparator)+fakeDockerDir)

	ins := testInstaller(srv)
	ins.InstallDir = installDir
	ins.PluginDir = t.TempDir()

	res, err := ins.Compose(context.Background(), p, "")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Kept, false))
	assert.Check(t, cmp.Equal(res.Version, DefaultComposeVersion))
	assert.Check(t, cmp.Equal(res.Replaced, "2.6.0"))
}

func TestComposeRefusesToInstallAVersionTooOld(t *testing.T) {
	_, err := Installer{}.Compose(context.Background(), Current(), "v2.17.0")
	assert.Check(t, cmp.ErrorContains(err, "compose v2.17.0 is older than "+MinComposeVersion))
}

func TestOlder(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"2.6.0", "2.24.0", true},
		{"v2.23.9", "2.24.0", true},
		{"2.24.0", "2.24.0", false},
		{"v2.24.1", "2.24.0", false},
		{"5.6.0", "2.24.0", false},
		{"2.30.0-desktop.1", "2.24.0", false},
		{"1.29.2", "2.24.0", true},
	} {
		assert.Check(t, cmp.Equal(older(tc.a, tc.b), tc.want), "%s < %s", tc.a, tc.b)
	}
}
