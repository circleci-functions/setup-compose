package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/circleci-functions/toolkit/bashenv"
	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-compose/compose"
	"github.com/circleci-functions/setup-compose/envvars"
	"github.com/circleci-functions/setup-compose/follow"
	"github.com/circleci-functions/setup-compose/install"
	"github.com/circleci-functions/setup-compose/ready"
)

type upCmd struct {
	Project projectFlags `embed:""`

	Services       string `help:"Services to start, comma or newline separated, with the services they depend on. Empty starts them all."`
	Build          bool   `help:"Build images before starting, even ones that already exist."`
	Pull           string `help:"When to pull images: always, missing, never or build. Empty leaves it to compose."`
	WaitTimeout    string `default:"5m" help:"How long to wait for every service to be healthy, or running if it has no healthcheck, and for its published ports to answer." example:"10m"`
	WaitPorts      bool   `default:"true" negatable:"" help:"Wait for the published ports of services without a healthcheck to answer, not just for their containers to start."`
	ExportEnv      bool   `default:"true" negatable:"" help:"Export COMPOSE_<SERVICE>_HOST and COMPOSE_<SERVICE>_PORT for each service with a published port, and the project's files, name and profiles, so later steps reach the services and can run docker compose on the same project."`
	LogsOnFail     bool   `default:"true" negatable:"" help:"Keep every service's logs, and store them as artifacts under compose-logs/<project> if the job fails."`
	ExitCodeFrom   string `help:"Run in the foreground until this service exits, stop the others, and exit with its status, for tests that run inside the project, as under setup_remote_docker. Waiting, exports and log collection are skipped."`
	ComposeVersion string `help:"Compose version to install, or latest. Empty keeps the Compose already installed, and installs ${compose_version} only if there is none."`
	InstallDir     string `hidden:"" help:"Link docker-compose into this directory instead of one already on PATH."`
	DownloadURL    string `hidden:"" help:"Base URL to download Compose from in place of github.com."`
	LogDir         string `hidden:"" help:"Directory to keep service logs in."`
}

func (c *upCmd) Run(ctx context.Context, client *sdk.Client) error {
	timeout, err := durationFlag("wait-timeout", c.WaitTimeout)
	if err != nil {
		return err
	}
	if !slices.Contains([]string{"", "always", "missing", "never", "build"}, c.Pull) {
		return fmt.Errorf("--pull=%q: want always, missing, never or build", c.Pull)
	}
	if err := c.ensureCompose(ctx); err != nil {
		return err
	}

	p := c.Project.project()
	services := compose.SplitList(c.Services)
	if c.ExitCodeFrom != "" {
		return p.Run(ctx, append(c.upArgs("--abort-on-container-exit", "--exit-code-from", c.ExitCodeFrom), services...)...)
	}

	deadline := time.Now().Add(timeout)
	upErr := p.Run(ctx, append(c.upArgs("--detach", "--wait", "--wait-timeout", seconds(timeout)), services...)...)
	if upErr != nil && settle(ctx, p, deadline) {
		upErr = nil
	}

	name, err := p.ResolvedName(ctx)
	if err != nil {
		return errors.Join(upErr, err)
	}
	if c.LogsOnFail {
		c.keepLogs(ctx, client, p, name)
	}
	if upErr != nil {
		showFailures(ctx, p)
		return upErr
	}

	containers, err := p.PS(ctx)
	if err != nil {
		return err
	}
	remote := remoteEngine()
	if c.WaitPorts {
		if err := waitPorts(ctx, containers, remote, deadline); err != nil {
			return err
		}
	}
	if c.ExportEnv {
		exportEnv(c.Project, containers, remote)
	}
	return nil
}

func (c *upCmd) upArgs(mode ...string) []string {
	args := append([]string{"up"}, mode...)
	if c.Build {
		args = append(args, "--build")
	}
	if c.Pull != "" {
		args = append(args, "--pull", c.Pull)
	}
	return args
}

// ensureCompose installs Compose when it is missing or a version is asked
// for. The Docker CLI itself has to be there already.
func (c *upCmd) ensureCompose(ctx context.Context) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("setup-compose needs the Docker CLI on PATH: use a machine executor, or an image that ships it, such as cimg/base, with setup_remote_docker")
	}
	i := install.New(c.InstallDir)
	if c.DownloadURL != "" {
		i.GitHubURL = c.DownloadURL
	}
	res, err := i.Compose(ctx, install.Current(), c.ComposeVersion)
	if err != nil {
		return fmt.Errorf("install compose: %w", err)
	}
	if res.Kept {
		fmt.Fprintf(os.Stderr, "setup-compose: using compose %s\n", res.Version)
	} else {
		fmt.Fprintf(os.Stderr, "setup-compose: installed compose %s\n", res.Version)
	}
	return nil
}

// keepLogs follows every service's logs into files, and registers them to be
// stored as artifacts if the job fails. It runs whether or not the services
// came up: `logs --follow` starts from each container's first line, and the
// logs of a service that failed to start are the ones worth having.
func (c *upCmd) keepLogs(ctx context.Context, client *sdk.Client, p compose.Project, name string) {
	services, err := p.Services(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup-compose: not keeping logs: %v\n", err)
		return
	}
	dir := cmp.Or(c.LogDir, filepath.Join(os.TempDir(), "setup-compose", name, "logs"))
	// Not tied to ctx: the followers must outlive this process.
	cmdFor := func(args ...string) *exec.Cmd { return p.Command(context.Background(), args...) } //nolint:contextcheck // see above
	if err := follow.Start(cmdFor, dir, services); err != nil {
		fmt.Fprintf(os.Stderr, "setup-compose: not keeping logs: %v\n", err)
		return
	}
	dest := "compose-logs/" + name
	switch err := client.AtTeardown().OnFail().StoreArtifacts(ctx, dir, dest); {
	case errors.Is(err, sdk.ErrNoServer):
		fmt.Fprintf(os.Stderr, "setup-compose: not in a CircleCI job, so service logs stay in %s\n", dir)
	case err != nil:
		fmt.Fprintf(os.Stderr, "setup-compose: service logs are in %s, but will not be stored: %v\n", dir, err)
	default:
		fmt.Fprintf(os.Stderr, "setup-compose: if the job fails, service logs are stored as artifacts under %s\n", dest)
	}
}

// settle finishes the wait compose gave up on when a one-shot service, such
// as a migration, exited 0: `up --wait` fails on any container that exits,
// unless another service waits for it to complete. It reports whether every
// container is then healthy, running without a healthcheck, or done.
func settle(ctx context.Context, p compose.Project, deadline time.Time) bool {
	cs, err := p.PS(ctx)
	if err != nil || !slices.ContainsFunc(cs, completed) {
		return false
	}
	for {
		pending := false
		for _, c := range cs {
			switch {
			case c.State == "exited" && c.ExitCode != 0, c.Health == "unhealthy":
				return false
			case !upToDate(c):
				pending = true
			}
		}
		if !pending {
			for _, c := range cs {
				if completed(c) {
					fmt.Fprintf(os.Stderr, "setup-compose: %s ran to completion\n", c.Name)
				}
			}
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(time.Second):
		}
		if cs, err = p.PS(ctx); err != nil {
			return false
		}
	}
}

func completed(c compose.Container) bool { return c.State == "exited" && c.ExitCode == 0 }

// upToDate reports whether a container is as far as `up --wait` takes it.
func upToDate(c compose.Container) bool {
	return c.Health == "healthy" || (c.Health == "" && c.State == "running") || completed(c)
}

// showFailures prints the end of the logs of each container that did not
// come up, so the reason is in the step's own output.
func showFailures(ctx context.Context, p compose.Project) {
	containers, err := p.PS(ctx)
	if err != nil {
		return
	}
	for _, c := range containers {
		if upToDate(c) {
			continue
		}
		fmt.Fprintf(os.Stderr, "\nsetup-compose: %s is %s%s; its last lines:\n", c.Name, c.State, healthSuffix(c.Health))
		_ = p.Run(ctx, "logs", "--no-color", "--tail", "50", c.Service)
	}
}

func healthSuffix(h string) string {
	if h == "" {
		return ""
	}
	return " (" + h + ")"
}

// waitPorts waits for the published TCP ports of running services that have
// no healthcheck, since compose only waits for their containers to start.
func waitPorts(ctx context.Context, containers []compose.Container, remote bool, deadline time.Time) error {
	var addrs, labels []string
	label := map[string]string{}
	for _, c := range containers {
		if c.Health != "" || c.State != "running" {
			continue
		}
		for _, pub := range envvars.Published(c.Publishers) {
			if pub.Protocol == "tcp" {
				addr := net.JoinHostPort(envvars.Host(pub.URL), strconv.Itoa(pub.PublishedPort))
				label[addr] = fmt.Sprintf("%s port %d on %s", c.Service, pub.TargetPort, addr)
				addrs = append(addrs, addr)
				labels = append(labels, label[addr])
			}
		}
	}
	if len(addrs) == 0 {
		return nil
	}
	if remote {
		fmt.Fprintln(os.Stderr, "setup-compose: not waiting for published ports, which are on the remote Docker engine")
		return nil
	}
	fmt.Fprintf(os.Stderr, "setup-compose: waiting for %s to answer\n", strings.Join(labels, ", "))
	wctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	pending := ready.Wait(wctx, addrs, 500*time.Millisecond)
	if len(pending) == 0 {
		return nil
	}
	for i, a := range pending {
		pending[i] = label[a]
	}
	return fmt.Errorf("nothing answered on %s within wait-timeout; is the service listening on that port?", strings.Join(pending, ", "))
}

// exportEnv writes the services' addresses and the project's settings to
// $BASH_ENV. It warns rather than fails, since they are a convenience.
func exportEnv(f projectFlags, containers []compose.Container, remote bool) {
	vars := projectVars(f)
	if remote {
		fmt.Fprintln(os.Stderr, "setup-compose: published ports are on the remote Docker engine, out of this job's reach, so no COMPOSE_<SERVICE>_* variables are exported")
	} else {
		for k, v := range envvars.Services(containers) {
			vars[k] = v
		}
	}
	if len(vars) == 0 {
		return
	}
	if err := bashenv.ExportEnvVars(vars); err != nil {
		fmt.Fprintf(os.Stderr, "setup-compose: not exporting variables: %v\n", err)
		return
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		fmt.Fprintf(os.Stderr, "setup-compose: exported %s=%s\n", k, vars[k])
	}
}

// projectVars sets compose's own variables to what the flags asked for, so
// `docker compose` in a later step, and the logs and down commands, target the
// same project without repeating them.
func projectVars(f projectFlags) map[string]string {
	vars := map[string]string{}
	if files := compose.SplitList(f.File); len(files) > 0 {
		vars["COMPOSE_FILE"] = strings.Join(absAll(files), string(os.PathListSeparator))
	}
	if f.ProjectName != "" {
		vars["COMPOSE_PROJECT_NAME"] = f.ProjectName
	}
	if profiles := compose.SplitList(f.Profile); len(profiles) > 0 {
		vars["COMPOSE_PROFILES"] = strings.Join(profiles, ",")
	}
	if envFiles := compose.SplitList(f.EnvFile); len(envFiles) > 0 {
		vars["COMPOSE_ENV_FILES"] = strings.Join(absAll(envFiles), ",")
	}
	return vars
}

func absAll(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		out[i] = abs
	}
	return out
}

// remoteEngine reports whether Docker runs somewhere other than this
// machine, as under setup_remote_docker, so published ports are not on
// loopback here.
func remoteEngine() bool {
	h := os.Getenv("DOCKER_HOST")
	if h == "" || strings.HasPrefix(h, "unix://") || strings.HasPrefix(h, "npipe://") {
		return false
	}
	rest := h[strings.Index(h, "://")+3:]
	host, _, err := net.SplitHostPort(rest)
	if err != nil {
		host = rest
	}
	return host != "localhost" && host != "127.0.0.1" && host != "::1"
}

// durationFlag parses a duration flag, which is a string so a config can say
// "10m": a kong time.Duration publishes as an integer.
func durationFlag(name, v string) (time.Duration, error) {
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("--%s=%q: want a duration such as 30s or 5m", name, v)
	}
	return d, nil
}

func seconds(d time.Duration) string {
	return strconv.Itoa(int(math.Ceil(d.Seconds())))
}
