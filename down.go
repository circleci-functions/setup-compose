package main

import (
	"context"
)

type downCmd struct {
	Project projectFlags `embed:""`

	Volumes bool   `default:"true" negatable:"" help:"Remove the project's named volumes and its containers' anonymous ones."`
	Timeout string `default:"10s" help:"How long to give containers to stop before killing them." example:"30s"`
}

func (c *downCmd) Run(ctx context.Context) error {
	timeout, err := durationFlag("timeout", c.Timeout)
	if err != nil {
		return err
	}
	args := []string{"down", "--remove-orphans", "--timeout", seconds(timeout)}
	if c.Volumes {
		args = append(args, "--volumes")
	}
	return c.Project.project().Run(ctx, args...)
}
