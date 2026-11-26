//go:build !windows

// Package clitest_test exercises the setup-compose CLI end to end against
// fakes: a shell script standing in for docker, and the toolkit's in-memory
// stand-in for the CircleCI job. Hermetic, so it runs under `task test` on
// every commit. A real Docker engine is covered by acceptance/.
package clitest_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/compiler"
	"github.com/circleci-functions/toolkit/testing/runner"
	"gotest.tools/v3/assert"
)

var binaryPath string

func TestMain(m *testing.M) {
	status, err := runTests(m)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	os.Exit(status)
}

func runTests(m *testing.M) (int, error) {
	p := compiler.NewParallel(1)
	defer p.Cleanup()

	p.Add(compiler.Work{
		Result: &binaryPath,
		Name:   "setup-compose",
		Target: "..",
		Source: ".",
	})
	if err := p.Run(context.Background()); err != nil {
		return 0, err
	}
	return m.Run(), nil
}

// fakeDocker answers the docker commands setup-compose runs, and records each
// one, a line per invocation, in calls.
const fakeDocker = `#!/bin/sh
echo "$*" >> "$FAKE_DOCKER_CALLS"
case "$*" in
  "compose version") echo "Docker Compose version v5.6.0" ;;
  "info --format "*) printf '%s\tUbuntu 24.04.3 LTS\n' "${FAKE_DOCKER_ENGINE_HOST:-$(hostname)}" ;;
  *" config --format json") echo '{"name":"app"}' ;;
  *" config --services") printf 'db\nweb\n' ;;
  *" ps --all --format json") cat "$FAKE_DOCKER_PS" ;;
  *" ps --all") echo "NAME STATUS" ;;
  *" up "*) exit "${FAKE_DOCKER_UP_EXIT:-0}" ;;
  *" logs "*) echo "a log line" ;;
esac
`

type job struct {
	t       *testing.T
	sandbox *sdktest.Sandbox
	dir     string
	calls   string
	ps      string
	env     map[string]string
}

// newJob returns a job whose PATH has the fake docker and the basic tools a
// shell script needs, and nothing else.
func newJob(t *testing.T) *job {
	t.Helper()
	bin := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(fakeDocker), 0o755)) //nolint:gosec // an executable stand-in
	j := &job{
		t:       t,
		sandbox: sdktest.NewSandbox(t),
		dir:     t.TempDir(),
		calls:   filepath.Join(t.TempDir(), "calls"),
		ps:      filepath.Join(t.TempDir(), "ps.json"),
	}
	j.env = map[string]string{
		"PATH":              bin + ":/usr/bin:/bin",
		"FAKE_DOCKER_CALLS": j.calls,
		"FAKE_DOCKER_PS":    j.ps,
	}
	j.setPS("")
	return j
}

func (j *job) setPS(lines ...string) {
	assert.NilError(j.t, os.WriteFile(j.ps, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
}

// run runs setup-compose in the job's working directory, and returns its
// combined output and exit code.
func (j *job) run(args ...string) (string, int) {
	j.t.Helper()
	env := j.sandbox.Env()
	for k, v := range j.env {
		env[k] = v
	}
	r := runner.New(j.t, binaryPath, runner.WithSandboxEnv(env), runner.WithCmdDir(j.dir))
	code, _ := r.Run(args...)
	return r.StdoutString() + r.StderrString(), code
}

// dockerCalls returns the docker command lines the run made, in order.
func (j *job) dockerCalls() []string {
	b, err := os.ReadFile(j.calls)
	if os.IsNotExist(err) {
		return nil
	}
	assert.NilError(j.t, err)
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func (j *job) bashEnv() string {
	b, err := os.ReadFile(j.sandbox.BashEnv)
	if os.IsNotExist(err) {
		return ""
	}
	assert.NilError(j.t, err)
	return string(b)
}
