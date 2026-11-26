//go:build acceptance

// Package acceptance_test runs setup-compose against a real Docker engine.
// Tagged out of `task test` because it needs one, and pulls images. The CLI
// contract itself is covered hermetically by clitest/.
package acceptance_test

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/circleci-functions/toolkit/bashenv"
	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/compiler"
	"github.com/circleci-functions/toolkit/testing/runner"
	"gotest.tools/v3/assert"
)

// binaryPathEnvVar names a prebuilt setup-compose to test, for an executor
// with no Go toolchain to compile one with. The smoke jobs set it, and pair it
// with a cross-compiled copy of this suite. A relative path is resolved here,
// since the suite runs the binary from a working directory of its own.
const binaryPathEnvVar = "SETUP_COMPOSE_BINARY"

//go:embed testdata
var testdata embed.FS

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
	if bin := os.Getenv(binaryPathEnvVar); bin != "" {
		abs, err := filepath.Abs(bin)
		if err != nil {
			return 0, fmt.Errorf("%s=%q: %w", binaryPathEnvVar, bin, err)
		}
		if _, err := os.Stat(abs); err != nil {
			return 0, fmt.Errorf("%s=%q: %w", binaryPathEnvVar, bin, err)
		}
		binaryPath = abs
		return m.Run(), nil
	}

	p := compiler.NewParallel(1)
	defer p.Cleanup()
	p.Add(compiler.Work{Result: &binaryPath, Name: "setup-compose", Target: "..", Source: "."})
	if err := p.Run(context.Background()); err != nil {
		return 0, err
	}
	return m.Run(), nil
}

// project is one compose project from testdata, in a directory of its own,
// under a name no other run shares.
type project struct {
	dir  string
	name string
}

// newProject writes testdata/<file> to dir as compose.yaml, and removes the
// project's containers, networks and volumes when the test ends. An empty dir
// means a temp one.
func newProject(t *testing.T, file, dir string) project {
	t.Helper()
	b, err := testdata.ReadFile("testdata/" + file)
	assert.NilError(t, err)
	if dir == "" {
		dir = t.TempDir()
	}
	assert.NilError(t, os.MkdirAll(dir, 0o755))
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), b, 0o600))
	return project{dir: dir, name: "sc-" + strings.TrimSuffix(file, ".yaml") + "-" + randomSuffix(t)}
}

func (p project) removeAtCleanup(t *testing.T) {
	t.Cleanup(func() {
		_ = exec.Command("docker", "compose", "--project-name", p.name, "down", "--volumes", "--remove-orphans", "--timeout", "1").Run()
	})
}

func randomSuffix(t *testing.T) string {
	b := make([]byte, 4)
	_, err := rand.Read(b)
	assert.NilError(t, err)
	return hex.EncodeToString(b)
}

// job runs setup-compose the way a step would: in the project's directory,
// with this process's environment (so it reaches the same Docker engine) and
// the sandbox's $BASH_ENV and job stand-in on top.
type job struct {
	sandbox *sdktest.Sandbox
	project project
	extra   map[string]string
}

func newJob(t *testing.T, p project) *job {
	t.Helper()
	return &job{sandbox: sdktest.NewSandbox(t), project: p, extra: map[string]string{}}
}

func (j *job) run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	env := j.sandbox.Env()
	// The real home, so docker finds its config, contexts and plugins.
	delete(env, "HOME")
	delete(env, "XDG_CACHE_HOME")
	maps.Copy(env, j.extra)
	r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars(), runner.WithSandboxEnv(env), runner.WithCmdDir(j.project.dir))
	args = append(args, "--project-name", j.project.name)
	_, err := r.Run(args...)
	out := r.StdoutString() + r.StderrString()
	t.Logf("setup-compose %s:\n%s", strings.Join(args, " "), out)
	return out, err
}

// exports returns what the run left in $BASH_ENV for later steps.
func (j *job) exports(t *testing.T) map[string]string {
	t.Helper()
	if _, err := os.Stat(j.sandbox.BashEnv); os.IsNotExist(err) {
		return map[string]string{}
	}
	all, err := bashenv.Vars(j.sandbox.BashEnv, map[string]string{})
	assert.NilError(t, err)
	return all
}

// remoteEngine reports whether Docker runs on another machine, as under
// setup_remote_docker, where published ports are out of this job's reach.
// The suite asks the engine where it runs, as setup-compose does, rather than
// reading DOCKER_HOST, since a remote engine may sit behind a local socket.
func remoteEngine() bool {
	out, err := exec.Command("docker", "info", "--format", "{{.Name}}\t{{.OperatingSystem}}").Output()
	if err != nil {
		return false
	}
	name, system, _ := strings.Cut(strings.TrimSpace(string(out)), "\t")
	host, _ := os.Hostname()
	short := func(h string) string { s, _, _ := strings.Cut(h, "."); return s }
	return !strings.Contains(system, "Docker Desktop") && !strings.EqualFold(short(name), short(host))
}

func skipOnRemoteEngine(t *testing.T) {
	t.Helper()
	if remoteEngine() {
		t.Skip("published ports are on the remote Docker engine")
	}
}
