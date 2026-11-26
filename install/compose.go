package install

import (
	"context"
	"path/filepath"
)

func (i Installer) Compose(ctx context.Context, p Platform, want string) (Result, error) {
	have, _ := installedVersion(ctx, "docker", "compose", "version")
	if have == "" {
		have, _ = installedVersion(ctx, "docker-compose", "version")
	}

	v, keep, err := chooseVersion(ctx, want, DefaultComposeVersion, have, func(ctx context.Context) (string, error) {
		return i.latestRelease(ctx, "docker/compose")
	})
	if err != nil {
		return Result{}, err
	}

	// An installed Compose is used as it is: everything runs it as
	// `docker compose`, so it needs no link of its own on PATH.
	if keep {
		return Result{Tool: "compose", Version: v, Kept: true}, nil
	}

	asset, err := p.composeAsset()
	if err != nil {
		return Result{}, err
	}
	base := i.GitHubURL + "/docker/compose/releases/download/" + v + "/"
	sum, err := i.releaseChecksum(ctx, base+"checksums.txt", asset)
	if err != nil {
		return Result{}, err
	}

	dest := filepath.Join(i.PluginDir, p.exe("docker-compose"))
	err = i.fetchVerified(ctx, base+asset, sum, func(f string) error {
		return copyExecutable(f, dest)
	})
	if err != nil {
		return Result{}, err
	}

	dir, err := i.link(ctx, i.PluginDir, v, "docker-compose")
	if err != nil {
		return Result{}, err
	}
	return Result{Tool: "compose", Version: v, Dir: dir}, nil
}
