package main

import (
	"os"

	"github.com/circleci-functions/setup-compose/compose"
)

// projectFlags pick the project, and are the same on every command.
type projectFlags struct {
	File             string `help:"Compose files, comma or newline separated. Empty uses COMPOSE_FILE, or compose.yaml in the working directory." example:"compose.yaml,compose.ci.yaml"`
	ProjectName      string `help:"Project name. Empty uses COMPOSE_PROJECT_NAME, or the name of the project directory."`
	ProjectDirectory string `help:"Directory relative paths in the compose files are resolved against. Empty is the first file's directory."`
	EnvFile          string `help:"Files of variables to interpolate into the compose files, comma or newline separated. Empty uses .env in the project directory."`
	Profile          string `help:"Profiles to enable, comma or newline separated." example:"ci"`
}

func (f projectFlags) project() compose.Project {
	return compose.Project{
		Files:     compose.SplitList(f.File),
		Name:      f.ProjectName,
		Directory: f.ProjectDirectory,
		EnvFiles:  compose.SplitList(f.EnvFile),
		Profiles:  compose.SplitList(f.Profile),
		Stdout:    os.Stderr,
		Stderr:    os.Stderr,
	}
}
