//go:build !windows

package follow

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/poll"
)

// TestStartWritesEachServiceToItsOwnFile stands a shell in for docker, so the
// test sees which arguments each follower got and that it keeps running after
// Start returns.
func TestStartWritesEachServiceToItsOwnFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	cmdFor := func(args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", `echo "$*"; sleep 0.2; echo later`, "sh", strings.Join(args, " "))
	}

	assert.NilError(t, Start(cmdFor, dir, []string{"db", "web"}))

	for _, svc := range []string{"db", "web"} {
		path := filepath.Join(dir, svc+".log")
		poll.WaitOn(t, func(poll.LogT) poll.Result {
			b, _ := os.ReadFile(path)
			if strings.Contains(string(b), "later") {
				return poll.Success()
			}
			return poll.Continue("%s has %q", path, b)
		}, poll.WithTimeout(5*time.Second))
		b, err := os.ReadFile(path)
		assert.NilError(t, err)
		assert.Check(t, cmp.Contains(string(b), "logs --follow --no-color --timestamps --no-log-prefix "+svc))
	}
}
