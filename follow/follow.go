// Package follow keeps writing each service's logs to a file after the step
// that started the services has finished.
package follow

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Start launches one `docker compose logs --follow` per service, each writing
// to <dir>/<service>.log, and leaves them running. They are detached from
// this process, so they outlive the step, and each stops once its service's
// containers are removed.
func Start(cmdFor func(args ...string) *exec.Cmd, dir string, services []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // logs are stored as artifacts anyway
		return err
	}
	for _, svc := range services {
		if err := start(cmdFor, filepath.Join(dir, svc+".log"), svc); err != nil {
			return fmt.Errorf("follow %s logs: %w", svc, err)
		}
	}
	return nil
}

func start(cmdFor func(args ...string) *exec.Cmd, path, service string) error {
	f, err := os.Create(path) //nolint:gosec // path is built from the log directory and a service name
	if err != nil {
		return err
	}
	defer f.Close()

	cmd := cmdFor("logs", "--follow", "--no-color", "--timestamps", "--no-log-prefix", service)
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.SysProcAttr = detached()
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
