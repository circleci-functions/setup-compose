package envvars

import (
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/circleci-functions/setup-compose/compose"
)

func TestServices(t *testing.T) {
	containers := []compose.Container{
		{Name: "app-db-1", Service: "db", Publishers: []compose.Publisher{
			{URL: "::", TargetPort: 5432, PublishedPort: 60763, Protocol: "tcp"},
			{URL: "0.0.0.0", TargetPort: 5432, PublishedPort: 60763, Protocol: "tcp"},
		}},
		{Name: "app-web-2", Service: "web", Publishers: []compose.Publisher{
			{URL: "0.0.0.0", TargetPort: 80, PublishedPort: 9000, Protocol: "tcp"},
		}},
		{Name: "app-web-1", Service: "web", Publishers: []compose.Publisher{
			{URL: "127.0.0.1", TargetPort: 443, PublishedPort: 8443, Protocol: "tcp"},
			{URL: "127.0.0.1", TargetPort: 80, PublishedPort: 8080, Protocol: "tcp"},
		}},
		{Name: "app-statsd-1", Service: "statsd", Publishers: []compose.Publisher{
			{URL: "127.0.0.1", TargetPort: 8125, PublishedPort: 8125, Protocol: "udp"},
		}},
		{Name: "app-migrate-1", Service: "migrate"},
		{Name: "app-cache-1", Service: "my-cache.v2", Publishers: []compose.Publisher{
			{TargetPort: 6379, Protocol: "tcp"}, // exposed, not published
		}},
	}

	assert.Check(t, cmp.DeepEqual(Services(containers), map[string]string{
		"COMPOSE_DB_HOST":      "127.0.0.1",
		"COMPOSE_DB_PORT":      "60763",
		"COMPOSE_DB_PORT_5432": "60763",

		"COMPOSE_WEB_HOST":     "127.0.0.1",
		"COMPOSE_WEB_PORT":     "8080",
		"COMPOSE_WEB_PORT_80":  "8080",
		"COMPOSE_WEB_PORT_443": "8443",

		"COMPOSE_STATSD_HOST":          "127.0.0.1",
		"COMPOSE_STATSD_PORT":          "8125",
		"COMPOSE_STATSD_PORT_8125_UDP": "8125",
	}))
}

func TestServicesPrefersTCPForThePort(t *testing.T) {
	got := Services([]compose.Container{{Name: "x-1", Service: "x", Publishers: []compose.Publisher{
		{URL: "0.0.0.0", TargetPort: 53, PublishedPort: 5353, Protocol: "udp"},
		{URL: "0.0.0.0", TargetPort: 8080, PublishedPort: 18080, Protocol: "tcp"},
	}}})
	assert.Check(t, cmp.Equal(got["COMPOSE_X_PORT"], "18080"))
	assert.Check(t, cmp.Equal(got["COMPOSE_X_PORT_53_UDP"], "5353"))
}

func TestName(t *testing.T) {
	for in, want := range map[string]string{
		"db":          "DB",
		"my-cache.v2": "MY_CACHE_V2",
		"api_1":       "API_1",
	} {
		assert.Check(t, cmp.Equal(Name(in), want))
	}
}
