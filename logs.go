package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-compose/compose"
)

type logsCmd struct {
	Project projectFlags `embed:""`

	Services    string `help:"Services whose logs to store, comma or newline separated. Empty stores them all." example:"api"`
	Destination string `default:"compose-logs" help:"Artifact path to store the logs under, one <service>.log each, plus ps.txt."`
	Dir         string `hidden:"" help:"Directory to write the logs to before storing them."`
}

func (c *logsCmd) Run(ctx context.Context, client *sdk.Client) error {
	p := c.Project.project()
	services := compose.SplitList(c.Services)
	if len(services) == 0 {
		var err error
		if services, err = p.Services(ctx); err != nil {
			return err
		}
	}

	dir := c.Dir
	if dir == "" {
		var err error
		if dir, err = os.MkdirTemp("", "setup-compose-logs-"); err != nil {
			return err
		}
	} else if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // logs are stored as artifacts anyway
		return err
	}

	if err := writeOutput(ctx, p, filepath.Join(dir, "ps.txt"), "ps", "--all"); err != nil {
		return err
	}
	for _, svc := range services {
		if err := writeOutput(ctx, p, filepath.Join(dir, svc+".log"), "logs", "--no-color", "--timestamps", "--no-log-prefix", svc); err != nil {
			return err
		}
	}

	dest := cmp.Or(c.Destination, "compose-logs")
	switch err := client.StoreArtifacts(ctx, dir, dest); {
	case errors.Is(err, sdk.ErrNoServer):
		fmt.Fprintf(os.Stderr, "setup-compose: not in a CircleCI job, so the logs stay in %s\n", dir)
	case err != nil:
		return fmt.Errorf("store logs: %w", err)
	default:
		fmt.Fprintf(os.Stderr, "setup-compose: stored the logs of %d services as artifacts under %s\n", len(services), dest)
	}
	return nil
}

func writeOutput(ctx context.Context, p compose.Project, path string, args ...string) error {
	out, err := p.Output(ctx, args...)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644) //nolint:gosec // logs are stored as artifacts anyway
}
