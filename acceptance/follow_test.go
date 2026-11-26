//go:build acceptance

package acceptance_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"gotest.tools/v3/assert"
)

// followPhaseEnvVar splits TestAcceptance_FollowsAfterTheStep across two job
// steps: "start" runs up and returns, "check" runs in a later step. Unset, both
// run in this process, which proves less: a step ending may stop more than
// the process that ran it.
const followPhaseEnvVar = "SETUP_COMPOSE_FOLLOW_PHASE"

// followDirEnvVar is where the two phases keep the project and its logs.
const followDirEnvVar = "SETUP_COMPOSE_FOLLOW_DIR"

func TestAcceptance_FollowsAfterTheStep(t *testing.T) {
	phase := os.Getenv(followPhaseEnvVar)
	dir := os.Getenv(followDirEnvVar)
	if phase != "" {
		sdktest.SkipUnlessInJob(t)
		assert.Assert(t, dir != "", "%s needs %s", followPhaseEnvVar, followDirEnvVar)
	} else {
		dir = t.TempDir()
	}
	projectDir := filepath.Join(dir, "project")
	logDir := filepath.Join(dir, "logs")
	nameFile := filepath.Join(dir, "name")

	var p project
	if phase == "" || phase == "start" {
		p = newProject(t, "ticker.yaml", projectDir)
		assert.NilError(t, os.WriteFile(nameFile, []byte(p.name), 0o600))
		out, err := newJob(t, p).run(t, "up", "--log-dir", logDir)
		assert.NilError(t, err, out)
		if phase == "start" {
			return
		}
	} else {
		name, err := os.ReadFile(nameFile)
		assert.NilError(t, err)
		p = project{dir: projectDir, name: string(name)}
	}
	p.removeAtCleanup(t)

	assertGrows(t, filepath.Join(logDir, "ticker.log"))
	out, err := newJob(t, p).run(t, "down", "--timeout", "1s")
	assert.NilError(t, err, out)
	assertStopsGrowing(t, filepath.Join(logDir, "ticker.log"))
}
