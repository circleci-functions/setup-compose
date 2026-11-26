// Command setup-compose starts a Docker Compose project for later steps in a
// CircleCI job, waits for its services to be ready, and collects their logs.
package main

import (
	"context"

	"github.com/alecthomas/kong"

	"github.com/circleci-functions/toolkit/functionyaml"
	"github.com/circleci-functions/toolkit/sdk"
)

type cli struct{}

func (c *cli) Run(context.Context, *sdk.Client) error {
	return nil
}

func main() {
	sdk.Run(run)
}

func run(ctx context.Context, client *sdk.Client) error {
	var flags cli
	kCtx := kong.Parse(&flags,
		kong.Name("setup-compose"),
		kong.Description("Start a Docker Compose project for later steps in the job, wait for its services to be ready, and keep their logs."),
		kong.UsageOnError(),
		functionyaml.Embed(),
	)
	kCtx.BindTo(ctx, (*context.Context)(nil))
	kCtx.Bind(client)
	return kCtx.Run()
}
