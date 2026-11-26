//go:build !windows

package clitest_test

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/poll"
)

// listen stands in for a service behind a published port, waiting for the
// client to speak as a database would.
func listen(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NilError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { time.Sleep(time.Second); _ = c.Close() }()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

// psLine is one running container as `docker compose ps --format json` lists it.
func psLine(name, service, health, publishers string) string {
	return fmt.Sprintf(`{"Name":%q,"Service":%q,"State":"running","Health":%q,"ExitCode":0,"Publishers":%s}`,
		name, service, health, publishers)
}

func TestUp(t *testing.T) {
	j := newJob(t)
	webPort := listen(t)
	j.setPS(
		psLine("app-db-1", "db", "healthy",
			`[{"URL":"0.0.0.0","TargetPort":5432,"PublishedPort":55432,"Protocol":"tcp"},{"URL":"::","TargetPort":5432,"PublishedPort":55432,"Protocol":"tcp"}]`),
		psLine("app-web-1", "web", "",
			fmt.Sprintf(`[{"URL":"127.0.0.1","TargetPort":80,"PublishedPort":%d,"Protocol":"tcp"}]`, webPort)),
	)
	logDir := filepath.Join(t.TempDir(), "logs")

	out, code := j.run("--file", "compose.yaml, compose.ci.yaml", "--profile", "ci", "--wait-timeout", "90s",
		"--services", "web", "--log-dir", logDir)
	assert.Equal(t, code, 0, out)

	calls := j.dockerCalls()
	assert.Check(t, cmp.Contains(calls,
		"compose --file compose.yaml --file compose.ci.yaml --profile ci up --detach --wait --wait-timeout 90 web"))

	t.Run("waits for ports without a healthcheck", func(t *testing.T) {
		assert.Check(t, cmp.Contains(out, fmt.Sprintf("waiting for web port 80 on 127.0.0.1:%d to answer", webPort)))
		assert.Check(t, !strings.Contains(out, "db port"), "db has a healthcheck, which compose waited for")
	})

	t.Run("exports the services and the project", func(t *testing.T) {
		env := j.bashEnv()
		wd, err := filepath.EvalSymlinks(j.dir)
		assert.NilError(t, err)
		for _, want := range []string{
			"export COMPOSE_DB_HOST='127.0.0.1'",
			"export COMPOSE_DB_PORT='55432'",
			"export COMPOSE_DB_PORT_5432='55432'",
			"export COMPOSE_WEB_HOST='127.0.0.1'",
			fmt.Sprintf("export COMPOSE_WEB_PORT='%d'", webPort),
			fmt.Sprintf("export COMPOSE_FILE='%s:%s'", filepath.Join(wd, "compose.yaml"), filepath.Join(wd, "compose.ci.yaml")),
			"export COMPOSE_PROFILES='ci'",
		} {
			assert.Check(t, cmp.Contains(env, want))
		}
		assert.Check(t, !strings.Contains(env, "COMPOSE_PROJECT_NAME"), "no project name was given")
	})

	t.Run("follows every service's logs", func(t *testing.T) {
		for _, svc := range []string{"db", "web"} {
			poll.WaitOn(t, func(poll.LogT) poll.Result {
				b, _ := os.ReadFile(filepath.Join(logDir, svc+".log"))
				if strings.Contains(string(b), "a log line") {
					return poll.Success()
				}
				return poll.Continue("%s.log has %q", svc, b)
			}, poll.WithTimeout(5*time.Second))
		}
		poll.WaitOn(t, func(poll.LogT) poll.Result {
			for _, svc := range []string{"db", "web"} {
				if !cmp.Contains(j.dockerCalls(), "compose --file compose.yaml --file compose.ci.yaml --profile ci logs --follow --no-color --timestamps --no-log-prefix "+svc)().Success() {
					return poll.Continue("no follower for %s yet", svc)
				}
			}
			return poll.Success()
		}, poll.WithTimeout(5*time.Second))
	})

	t.Run("stores the logs only if the job fails", func(t *testing.T) {
		assert.Check(t, cmp.DeepEqual(j.sandbox.Cache.PendingArtifactStores(), []string{logDir}))
		j.sandbox.Cache.EndJobAfterFailure(t)
		stored := j.sandbox.Cache.StoredArtifacts()
		assert.Assert(t, cmp.Len(stored, 1))
		assert.Check(t, cmp.Equal(stored[0].Destination, "compose-logs/app"))
	})
}

func TestUpLogsWhyAServiceFailed(t *testing.T) {
	j := newJob(t)
	j.env["FAKE_DOCKER_UP_EXIT"] = "1"
	j.setPS(
		psLine("app-db-1", "db", "healthy", "null"),
		psLine("app-web-1", "web", "unhealthy", "null"),
	)

	out, code := j.run("--log-dir", t.TempDir())
	assert.Check(t, code != 0, out)
	assert.Check(t, cmp.Contains(out, "app-web-1 is running (unhealthy); its last lines:"))
	assert.Check(t, cmp.Contains(j.dockerCalls(), "compose logs --no-color --tail 50 web"))
	assert.Check(t, !strings.Contains(out, "app-db-1 is"), "db is healthy")
	assert.Check(t, cmp.Len(j.sandbox.Cache.PendingArtifactStores(), 1), "the logs are kept for a failed start too")
	assert.Check(t, cmp.Equal(j.bashEnv(), ""), "nothing is exported for services that did not start")
}

func TestUpExitCodeFrom(t *testing.T) {
	j := newJob(t)
	out, code := j.run("--exit-code-from", "test", "--build", "--pull", "always")
	assert.Equal(t, code, 0, out)
	assert.Check(t, cmp.DeepEqual(j.dockerCalls(), []string{
		"compose version",
		"compose up --abort-on-container-exit --exit-code-from test --build --pull always",
	}))
	assert.Check(t, cmp.Len(j.sandbox.Cache.PendingArtifactStores(), 0))

	j = newJob(t)
	j.env["FAKE_DOCKER_UP_EXIT"] = "3"
	out, code = j.run("--exit-code-from", "test")
	assert.Check(t, code != 0, "the test service's failure fails the step: %s", out)
}

func TestUpOnARemoteEngine(t *testing.T) {
	j := newJob(t)
	j.env["DOCKER_HOST"] = "tcp://10.0.0.5:2376"
	j.setPS(
		psLine("app-db-1", "db", "healthy", `[{"URL":"0.0.0.0","TargetPort":5432,"PublishedPort":5432,"Protocol":"tcp"}]`),
		psLine("app-web-1", "web", "", `[{"URL":"0.0.0.0","TargetPort":80,"PublishedPort":1,"Protocol":"tcp"}]`),
	)

	out, code := j.run("--project-name", "app", "--log-dir", t.TempDir())
	assert.Equal(t, code, 0, out)
	assert.Check(t, cmp.Contains(out, "not waiting for published ports, which are on the remote Docker engine"))
	env := j.bashEnv()
	assert.Check(t, !strings.Contains(env, "COMPOSE_DB_"), env)
	assert.Check(t, cmp.Contains(env, "export COMPOSE_PROJECT_NAME='app'"))
}

func TestUpWithNothingToExport(t *testing.T) {
	j := newJob(t)
	out, code := j.run("--no-export-env", "--no-logs-on-fail")
	assert.Equal(t, code, 0, out)
	assert.Check(t, cmp.Equal(j.bashEnv(), ""))
	assert.Check(t, cmp.Len(j.sandbox.Cache.PendingArtifactStores(), 0))
}

func TestUpRejectsBadFlagsBeforeRunningAnything(t *testing.T) {
	for _, args := range [][]string{
		{"--pull", "sometimes"},
		{"--wait-timeout", "5"},
		{"--wait-timeout", "-1m"},
	} {
		j := newJob(t)
		out, code := j.run(args...)
		assert.Check(t, code != 0, "%v: %s", args, out)
		assert.Check(t, cmp.Contains(out, args[0]+"="), "%v: %s", args, out)
		assert.Check(t, cmp.Len(j.dockerCalls(), 0), "%v", args)
	}
}

func TestUpNeedsTheDockerCLI(t *testing.T) {
	j := newJob(t)
	j.env["PATH"] = "/usr/bin:/bin"
	if _, err := os.Stat("/usr/bin/docker"); err == nil {
		t.Skip("this machine has docker in /usr/bin")
	}
	out, code := j.run()
	assert.Check(t, code != 0)
	assert.Check(t, cmp.Contains(out, "needs the Docker CLI on PATH"))
}

// TestUpCarriesOnPastAOneShotService covers a migration that exits 0 with
// nothing waiting on it, which fails compose's own wait.
func TestUpCarriesOnPastAOneShotService(t *testing.T) {
	for _, tc := range []struct {
		exit int
		pass bool
	}{{0, true}, {1, false}} {
		t.Run(fmt.Sprintf("exit %d", tc.exit), func(t *testing.T) {
			j := newJob(t)
			j.env["FAKE_DOCKER_UP_EXIT"] = "1"
			j.setPS(
				psLine("app-db-1", "db", "healthy", "null"),
				fmt.Sprintf(`{"Name":"app-migrate-1","Service":"migrate","State":"exited","Health":"","ExitCode":%d,"Publishers":null}`, tc.exit),
			)
			out, code := j.run("--wait-timeout", "5s", "--no-logs-on-fail")
			if tc.pass {
				assert.Check(t, cmp.Equal(code, 0), out)
				assert.Check(t, cmp.Contains(out, "app-migrate-1 ran to completion"))
			} else {
				assert.Check(t, code != 0, out)
				assert.Check(t, cmp.Contains(out, "app-migrate-1 is exited"))
			}
		})
	}
}
