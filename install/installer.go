package install

import (
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Installer struct {
	GitHubURL        string
	InstallDir       string
	PluginDir        string
	HTTP             *http.Client
	SystemPluginDirs []string
	// RetryWait is the first wait between download attempts, doubling after each. Zero means 1s.
	RetryWait time.Duration
}

func New(linkDir string) Installer {
	pluginDir := filepath.Join(pluginDirBase(), "cli-plugins")
	return Installer{
		GitHubURL:  "https://github.com",
		InstallDir: linkDir,
		PluginDir:  pluginDir,
		HTTP:       &http.Client{Timeout: 10 * time.Minute},
		SystemPluginDirs: []string{
			"/usr/local/lib/docker/cli-plugins",
			"/usr/local/libexec/docker/cli-plugins",
			"/usr/lib/docker/cli-plugins",
			"/usr/libexec/docker/cli-plugins",
		},
	}
}

func pluginDirBase() string {
	if dc := os.Getenv("DOCKER_CONFIG"); dc != "" {
		return dc
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".docker"
	}
	return filepath.Join(home, ".docker")
}
