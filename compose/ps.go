package compose

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Container is one entry of `docker compose ps --format json`.
type Container struct {
	Name       string      `json:"Name"`
	Service    string      `json:"Service"`
	State      string      `json:"State"`
	Health     string      `json:"Health"`
	ExitCode   int         `json:"ExitCode"`
	Publishers []Publisher `json:"Publishers"`
}

// Publisher is one published port of a container.
type Publisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
	Protocol      string `json:"Protocol"`
}

// ParsePS reads `docker compose ps --format json`, which is one JSON array
// before compose 2.21 and one object per line from then on.
func ParsePS(out []byte) ([]Container, error) {
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, nil
	}
	if out[0] == '[' {
		var cs []Container
		if err := json.Unmarshal(out, &cs); err != nil {
			return nil, fmt.Errorf("parse docker compose ps: %w", err)
		}
		return cs, nil
	}
	var cs []Container
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var c Container
		if err := dec.Decode(&c); err != nil {
			return nil, fmt.Errorf("parse docker compose ps: %w", err)
		}
		cs = append(cs, c)
	}
	return cs, nil
}
