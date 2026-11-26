//go:build !windows

package clitest_test

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestLogs(t *testing.T) {
	j := newJob(t)
	dir := filepath.Join(t.TempDir(), "out")

	out, code := j.run("logs", "--project-name", "app", "--dir", dir)
	assert.Equal(t, code, 0, out)

	for _, f := range []string{"ps.txt", "db.log", "web.log"} {
		_, err := os.Stat(filepath.Join(dir, f))
		assert.Check(t, err, "%s", f)
	}
	assert.Check(t, cmp.Contains(j.dockerCalls(), "compose --project-name app logs --no-color --timestamps --no-log-prefix web"))
	assert.Check(t, cmp.DeepEqual(j.sandbox.Cache.StoredArtifacts()[0].Destination, "compose-logs"))
	assert.Check(t, cmp.Equal(j.sandbox.Cache.StoredArtifacts()[0].Path, dir))
}

func TestLogsOfSomeServices(t *testing.T) {
	j := newJob(t)
	dir := t.TempDir()
	out, code := j.run("logs", "--services", "web", "--destination", "logs/after-tests", "--dir", dir)
	assert.Equal(t, code, 0, out)
	_, err := os.Stat(filepath.Join(dir, "db.log"))
	assert.Check(t, os.IsNotExist(err), "only web was asked for")
	assert.Check(t, cmp.Equal(j.sandbox.Cache.StoredArtifacts()[0].Destination, "logs/after-tests"))
}

func TestDown(t *testing.T) {
	j := newJob(t)
	out, code := j.run("down")
	assert.Equal(t, code, 0, out)
	assert.Check(t, cmp.DeepEqual(j.dockerCalls(), []string{"compose down --remove-orphans --timeout 10 --volumes"}))

	j = newJob(t)
	out, code = j.run("down", "--file", "ci.yaml", "--no-volumes", "--timeout", "1m30s")
	assert.Equal(t, code, 0, out)
	assert.Check(t, cmp.DeepEqual(j.dockerCalls(), []string{"compose --file ci.yaml down --remove-orphans --timeout 90"}))
}
