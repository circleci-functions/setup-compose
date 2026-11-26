package install

import (
	"fmt"
	"runtime"
)

// Platform holds the OS and architecture identifiers.
type Platform struct{ OS, Arch string }

// Current returns the platform this binary runs on.
func Current() Platform {
	return Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

var (
	goOS      = map[string]string{"linux": "linux", "darwin": "darwin", "windows": "windows"}
	unameArch = map[string]string{"amd64": "x86_64", "arm64": "aarch64"}
)

func (p Platform) pick(osNames, archNames map[string]string) (string, string, error) {
	goos, ok := osNames[p.OS]
	if !ok {
		return "", "", fmt.Errorf("unsupported platform %s/%s", p.OS, p.Arch)
	}
	arch, ok := archNames[p.Arch]
	if !ok {
		return "", "", fmt.Errorf("unsupported platform %s/%s", p.OS, p.Arch)
	}
	return goos, arch, nil
}

func (p Platform) exe(name string) string {
	if p.OS == "windows" {
		return name + ".exe"
	}
	return name
}

func (p Platform) composeAsset() (string, error) {
	goos, arch, err := p.pick(goOS, unameArch)
	if err != nil {
		return "", err
	}
	return p.exe(fmt.Sprintf("docker-compose-%s-%s", goos, arch)), nil
}
