// Package envvars names the variables that tell later steps where each
// service's published ports are.
package envvars

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/circleci-functions/setup-compose/compose"
)

// Prefix starts every exported service variable, so none can clash with a
// variable an application already reads under the service's own name.
const Prefix = "COMPOSE_"

// Services returns, for each service with a published port:
//
//	COMPOSE_<SERVICE>_HOST           the address to connect to
//	COMPOSE_<SERVICE>_PORT           the first published port, TCP if any
//	COMPOSE_<SERVICE>_PORT_<TARGET>  each published TCP port by container port
//	COMPOSE_<SERVICE>_PORT_<TARGET>_UDP  and each UDP one
//
// A scaled service is described by its first container.
func Services(containers []compose.Container) map[string]string {
	vars := map[string]string{}
	seen := map[string]bool{}
	for _, c := range sortedByName(containers) {
		pubs := published(c.Publishers)
		if len(pubs) == 0 || seen[c.Service] {
			continue
		}
		seen[c.Service] = true
		base := Prefix + Name(c.Service)
		vars[base+"_HOST"] = host(pubs[0].URL)
		vars[base+"_PORT"] = strconv.Itoa(pubs[0].PublishedPort)
		for _, p := range pubs {
			key := base + "_PORT_" + strconv.Itoa(p.TargetPort)
			if p.Protocol == "udp" {
				key += "_UDP"
			}
			vars[key] = strconv.Itoa(p.PublishedPort)
		}
	}
	return vars
}

// Name turns a service name into the part of a variable name that stands for
// it: upper-cased, with anything but letters and digits replaced by _.
func Name(service string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(service) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// published returns a container's published ports once each, preferring the
// IPv4 binding of a port published on both, TCP before UDP, then by
// container port.
func published(pubs []compose.Publisher) []compose.Publisher {
	byPort := map[string]compose.Publisher{}
	for _, p := range pubs {
		if p.PublishedPort == 0 {
			continue
		}
		key := p.Protocol + "/" + strconv.Itoa(p.TargetPort)
		if have, ok := byPort[key]; !ok || (isIPv6(have.URL) && !isIPv6(p.URL)) {
			byPort[key] = p
		}
	}
	out := make([]compose.Publisher, 0, len(byPort))
	for _, p := range byPort {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b compose.Publisher) int {
		if a.Protocol != b.Protocol {
			if a.Protocol == "tcp" {
				return -1
			}
			if b.Protocol == "tcp" {
				return 1
			}
		}
		return cmp.Or(cmp.Compare(a.TargetPort, b.TargetPort), strings.Compare(a.Protocol, b.Protocol))
	})
	return out
}

// host returns the address to reach a binding at. A port bound on every
// interface is reached on loopback, by address rather than as localhost,
// which some clients resolve to ::1 first.
func host(url string) string {
	switch url {
	case "", "0.0.0.0", "::":
		return "127.0.0.1"
	}
	return url
}

func isIPv6(url string) bool { return strings.Contains(url, ":") }

func sortedByName(cs []compose.Container) []compose.Container {
	out := slices.Clone(cs)
	slices.SortFunc(out, func(a, b compose.Container) int { return strings.Compare(a.Name, b.Name) })
	return out
}
