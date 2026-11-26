// Command setup-compose starts a Docker Compose project for later steps in a
// CircleCI job, waits for its services to be ready, keeps their logs, and
// tears it down.
//
// Supported platforms: linux. macOS executors have no Docker engine, and
// Windows ones run Windows containers only.
package main

import (
	"context"

	"github.com/alecthomas/kong"

	"github.com/circleci-functions/toolkit/functionyaml"
	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-compose/install"
)

type cli struct {
	Up   upCmd   `cmd:"" default:"withargs" help:"Start the services in dependency order, wait for them to be healthy, and export their addresses. Runs when no command is given."`
	Logs logsCmd `cmd:"" help:"Store every service's logs as artifacts now."`
	Down downCmd `cmd:"" help:"Stop and remove the project's containers, networks and volumes."`
}

func main() {
	sdk.Run(run)
}

func run(ctx context.Context, client *sdk.Client) error {
	var flags cli
	kCtx := kong.Parse(&flags,
		kong.Name("setup-compose"),
		kong.Description("Start a Docker Compose project for later steps in the job: start its services in dependency order, wait for them to be healthy, export their addresses, keep their logs if the job fails, and tear it down. Use instead of docker compose run steps with sleeps and wait loops."),
		kong.UsageOnError(),
		kong.Vars{"compose_version": install.DefaultComposeVersion},
		functionyaml.Embed(),
	)
	kCtx.BindTo(ctx, (*context.Context)(nil))
	kCtx.Bind(client)
	return kCtx.Run()
}
