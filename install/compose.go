package install

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// MinComposeVersion is the oldest Compose setup-compose works with: the first
// with every flag and variable it uses, such as `up --wait-timeout` and
// COMPOSE_ENV_FILES.
const MinComposeVersion = "2.24.0"

// Compose installs the Compose CLI plugin, unless the one `docker compose`
// already runs will do: with no version asked for, any at least
// MinComposeVersion; otherwise, exactly the one asked for. Result.Replaced
// names an installed version too old to keep.
func (i Installer) Compose(ctx context.Context, p Platform, want string) (Result, error) {
	if want != "" && want != "latest" && older(want, MinComposeVersion) {
		return Result{}, fmt.Errorf("compose %s is older than %s, the oldest setup-compose works with", want, MinComposeVersion)
	}
	// Only the plugin counts: a standalone docker-compose does not make
	// `docker compose` work.
	have, _ := installedVersion(ctx, "docker", "compose", "version")
	var replaced string
	if have != "" && older(have, MinComposeVersion) {
		replaced, have = have, ""
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
	return Result{Tool: "compose", Version: v, Dir: dir, Replaced: replaced}, nil
}

// older reports whether version a is older than b, comparing major, minor
// and patch numbers and ignoring a leading v and any suffix.
func older(a, b string) bool {
	pa, pb := parts(a), parts(b)
	for k := range pa {
		if pa[k] != pb[k] {
			return pa[k] < pb[k]
		}
	}
	return false
}

func parts(v string) [3]int {
	var out [3]int
	m := versionRe.FindString(strings.TrimPrefix(v, "v"))
	for k, f := range strings.SplitN(m, ".", 3) {
		out[k], _ = strconv.Atoi(f)
	}
	return out
}
