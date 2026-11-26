//go:build acceptance

package acceptance_test

import (
	"bufio"
	"net"
	"net/http"
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

// TestAcceptance_UpLogsDown runs the three commands in the order a job would.
func TestAcceptance_UpLogsDown(t *testing.T) {
	skipOnRemoteEngine(t)
	p := newProject(t, "services.yaml", "")
	p.removeAtCleanup(t)
	j := newJob(t, p)
	logDir := filepath.Join(t.TempDir(), "followed")

	out, err := j.run(t, "up", "--wait-timeout", "3m", "--log-dir", logDir)
	assert.NilError(t, err, out)
	env := j.exports(t)

	t.Run("ExportsWhereEachServiceIs", func(t *testing.T) {
		for _, svc := range []string{"CACHE", "WEB", "SLOW"} {
			assert.Check(t, cmp.Equal(env["COMPOSE_"+svc+"_HOST"], "127.0.0.1"), svc)
			assert.Check(t, env["COMPOSE_"+svc+"_PORT"] != "", svc)
		}
		assert.Check(t, cmp.Equal(env["COMPOSE_WEB_PORT_80"], env["COMPOSE_WEB_PORT"]))
		assert.Check(t, cmp.Equal(env["COMPOSE_PROJECT_NAME"], p.name))
		_, hasTicker := env["COMPOSE_TICKER_HOST"]
		assert.Check(t, !hasTicker, "ticker publishes nothing")
	})

	t.Run("ServicesAnswerOnTheExportedPorts", func(t *testing.T) {
		resp, err := http.Get("http://" + net.JoinHostPort(env["COMPOSE_WEB_HOST"], env["COMPOSE_WEB_PORT"]))
		assert.NilError(t, err)
		_ = resp.Body.Close()
		assert.Check(t, cmp.Equal(resp.StatusCode, http.StatusOK))

		assert.Check(t, cmp.Equal(roundTrip(t, env["COMPOSE_CACHE_HOST"], env["COMPOSE_CACHE_PORT"], "PING\r\n"), "+PONG"))
	})

	t.Run("WaitedForTheSlowServer", func(t *testing.T) {
		// slow listens 5s after it starts and has no healthcheck, so only the
		// port wait stands between `up` returning and this answer.
		assert.Check(t, cmp.Contains(out, "waiting for"))
		assert.Check(t, cmp.Equal(roundTrip(t, env["COMPOSE_SLOW_HOST"], env["COMPOSE_SLOW_PORT"], ""), "hello"))
	})

	t.Run("FollowsLogsAfterItExits", func(t *testing.T) {
		assertGrows(t, filepath.Join(logDir, "ticker.log"))
		b, err := os.ReadFile(filepath.Join(logDir, "migrate.log"))
		assert.NilError(t, err)
		assert.Check(t, cmp.Contains(string(b), "migrated"))
	})

	t.Run("LogsStoresEveryService", func(t *testing.T) {
		dir := t.TempDir()
		out, err := j.run(t, "logs", "--dir", dir)
		assert.NilError(t, err, out)
		for _, svc := range []string{"cache", "web", "migrate", "slow", "ticker"} {
			_, err := os.Stat(filepath.Join(dir, svc+".log"))
			assert.Check(t, err, svc)
		}
		ps, err := os.ReadFile(filepath.Join(dir, "ps.txt"))
		assert.NilError(t, err)
		assert.Check(t, cmp.Contains(string(ps), p.name+"-web-1"))
		assert.Check(t, cmp.Len(j.sandbox.Cache.StoredArtifacts(), 1))
	})

	t.Run("DownRemovesEverything", func(t *testing.T) {
		out, err := j.run(t, "down", "--timeout", "2s")
		assert.NilError(t, err, out)
		left, err := exec.Command("docker", "ps", "--all", "--quiet", "--filter", "label=com.docker.compose.project="+p.name).Output()
		assert.NilError(t, err)
		assert.Check(t, cmp.Equal(strings.TrimSpace(string(left)), ""))
		assertStopsGrowing(t, filepath.Join(logDir, "ticker.log"))
	})
}

func TestAcceptance_UpFailsOnAPortNothingServes(t *testing.T) {
	skipOnRemoteEngine(t)
	p := newProject(t, "unserved.yaml", "")
	p.removeAtCleanup(t)
	j := newJob(t, p)

	out, err := j.run(t, "up", "--wait-timeout", "8s", "--no-logs-on-fail")
	assert.Check(t, err != nil, out)
	assert.Check(t, cmp.Contains(out, "nothing answered on quiet port 80 on 127.0.0.1:"))
}

func TestAcceptance_UpShowsWhyAServiceIsUnhealthy(t *testing.T) {
	p := newProject(t, "unhealthy.yaml", "")
	p.removeAtCleanup(t)
	j := newJob(t, p)

	out, err := j.run(t, "up", "--wait-timeout", "30s", "--log-dir", t.TempDir())
	assert.Check(t, err != nil, out)
	assert.Check(t, cmp.Contains(out, p.name+"-sick-1 is running (unhealthy); its last lines:"))
	assert.Check(t, cmp.Contains(out, "starting up"))

	j.sandbox.Cache.EndJobAfterFailure(t)
	stored := j.sandbox.Cache.StoredArtifacts()
	assert.Assert(t, cmp.Len(stored, 1))
	assert.Check(t, cmp.Equal(stored[0].Destination, "compose-logs/"+p.name))
	b, err := os.ReadFile(filepath.Join(stored[0].Path, "sick.log"))
	assert.NilError(t, err)
	assert.Check(t, cmp.Contains(string(b), "starting up"))
}

// TestAcceptance_ExitCodeFrom runs tests inside the project, which works on a
// remote engine as well as a local one.
func TestAcceptance_ExitCodeFrom(t *testing.T) {
	for _, tc := range []struct {
		exit string
		pass bool
	}{{"0", true}, {"7", false}} {
		t.Run("exit "+tc.exit, func(t *testing.T) {
			p := newProject(t, "tests-inside.yaml", "")
			p.removeAtCleanup(t)
			j := newJob(t, p)
			j.extra["TEST_EXIT"] = tc.exit

			out, err := j.run(t, "up", "--exit-code-from", "test")
			assert.Check(t, cmp.Contains(out, "reached web"))
			if tc.pass {
				assert.Check(t, err, out)
			} else {
				assert.Check(t, err != nil, out)
			}
			assert.Check(t, cmp.Len(j.exports(t), 0))
		})
	}
}

func TestAcceptance_RemoteEngine(t *testing.T) {
	if !remoteEngine() {
		t.Skip("needs a remote Docker engine, as under setup_remote_docker")
	}
	p := newProject(t, "services.yaml", "")
	p.removeAtCleanup(t)
	j := newJob(t, p)

	out, err := j.run(t, "up", "--wait-timeout", "3m", "--log-dir", t.TempDir())
	assert.NilError(t, err, out)
	assert.Check(t, cmp.Contains(out, "published ports are on the remote Docker engine"))
	env := j.exports(t)
	_, hasWeb := env["COMPOSE_WEB_HOST"]
	assert.Check(t, !hasWeb)
	assert.Check(t, cmp.Equal(env["COMPOSE_PROJECT_NAME"], p.name))
}

// roundTrip sends msg, if any, and returns the first line that comes back.
func roundTrip(t *testing.T, host, port, msg string) string {
	t.Helper()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	assert.NilError(t, err)
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if msg != "" {
		_, err = conn.Write([]byte(msg))
		assert.NilError(t, err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	assert.NilError(t, err)
	return strings.TrimSpace(line)
}

func size(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func assertGrows(t *testing.T, path string) {
	t.Helper()
	start := size(path)
	poll.WaitOn(t, func(poll.LogT) poll.Result {
		if size(path) > start {
			return poll.Success()
		}
		return poll.Continue("%s is still %d bytes", path, start)
	}, poll.WithTimeout(10*time.Second))
}

func assertStopsGrowing(t *testing.T, path string) {
	t.Helper()
	poll.WaitOn(t, func(poll.LogT) poll.Result {
		before := size(path)
		time.Sleep(2 * time.Second)
		if size(path) == before {
			return poll.Success()
		}
		return poll.Continue("%s is still growing", path)
	}, poll.WithTimeout(20*time.Second))
}
