//go:build acceptance

package acceptance_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/runner"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/circleci-functions/setup-compose/install"
)

// TestAcceptance_InstallsComposeOntoTheRealPath lets setup-compose install
// Compose where it chooses on this executor, then finds it from a later step.
// It changes the executor, so it only runs inside a job.
func TestAcceptance_InstallsComposeOntoTheRealPath(t *testing.T) {
	sdktest.SkipUnlessInJob(t)
	p := newProject(t, "ticker.yaml", "")
	p.removeAtCleanup(t)
	want := strings.TrimPrefix(install.DefaultComposeVersion, "v")

	r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars(), runner.WithCmdDir(p.dir),
		runner.WithSandboxEnv(map[string]string{"BASH_ENV": filepath.Join(t.TempDir(), "bash_env")}))
	_, err := r.Run("up", "--project-name", p.name, "--compose-version", install.DefaultComposeVersion)
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, out)
	assert.Check(t, cmp.Contains(out, "compose "+install.DefaultComposeVersion))

	for _, cmd := range [][]string{{"docker", "compose", "version"}, {"docker-compose", "version"}} {
		got, err := sdktest.LaterStep(t, cmd[0], cmd[1:]...)
		assert.NilError(t, err, got)
		assert.Check(t, cmp.Contains(got, want), "%v", cmd)
	}
	path, err := exec.LookPath("docker-compose")
	assert.NilError(t, err)
	t.Logf("docker-compose resolves to %s", path)
}
