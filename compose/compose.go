// Package compose runs `docker compose` against one project.
package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Project names the files, project and profiles every command targets. Empty
// fields are left to compose, which falls back to its COMPOSE_* environment
// variables and then to its own defaults.
type Project struct {
	Files     []string
	Name      string
	Directory string
	EnvFiles  []string
	Profiles  []string

	// Docker is the docker binary to run. Empty means "docker" from PATH.
	Docker string
	Stdout io.Writer
	Stderr io.Writer
}

// Args returns the full argument list for `docker compose <args>`.
func (p Project) Args(args ...string) []string {
	out := []string{"compose"}
	for _, f := range p.Files {
		out = append(out, "--file", f)
	}
	if p.Name != "" {
		out = append(out, "--project-name", p.Name)
	}
	if p.Directory != "" {
		out = append(out, "--project-directory", p.Directory)
	}
	for _, f := range p.EnvFiles {
		out = append(out, "--env-file", f)
	}
	for _, pr := range p.Profiles {
		out = append(out, "--profile", pr)
	}
	return append(out, args...)
}

// Command returns `docker compose <args>` for this project, unstarted.
func (p Project) Command(ctx context.Context, args ...string) *exec.Cmd {
	docker := p.Docker
	if docker == "" {
		docker = "docker"
	}
	return exec.CommandContext(ctx, docker, p.Args(args...)...)
}

// Run runs a compose command, streaming its output to Stdout and Stderr.
func (p Project) Run(ctx context.Context, args ...string) error {
	cmd := p.Command(ctx, args...)
	cmd.Stdout = writerOr(p.Stdout, os.Stderr)
	cmd.Stderr = writerOr(p.Stderr, os.Stderr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// Output runs a compose command and returns its standard output. Standard
// error goes to Stderr, and is quoted in the error if the command fails.
func (p Project) Output(ctx context.Context, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := p.Command(ctx, args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Services returns the names of the services the project would start, with
// its profiles applied.
func (p Project) Services(ctx context.Context) ([]string, error) {
	out, err := p.Output(ctx, "config", "--services")
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// ResolvedName returns the project name compose settles on, which is the
// directory's name when nothing sets one.
func (p Project) ResolvedName(ctx context.Context) (string, error) {
	out, err := p.Output(ctx, "config", "--format", "json")
	if err != nil {
		return "", err
	}
	var cfg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &cfg); err != nil {
		return "", fmt.Errorf("parse docker compose config: %w", err)
	}
	return cfg.Name, nil
}

// PS returns the project's containers, stopped ones included.
func (p Project) PS(ctx context.Context) ([]Container, error) {
	out, err := p.Output(ctx, "ps", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}
	return ParsePS(out)
}

func writerOr(w, fallback io.Writer) io.Writer {
	if w != nil {
		return w
	}
	return fallback
}
