package compose

import (
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestArgs(t *testing.T) {
	p := Project{
		Files:     []string{"a.yaml", "b.yaml"},
		Name:      "app",
		Directory: "deploy",
		EnvFiles:  []string{".env.ci"},
		Profiles:  []string{"ci", "db"},
	}
	assert.Check(t, cmp.DeepEqual(p.Args("up", "--wait"), []string{
		"compose", "--file", "a.yaml", "--file", "b.yaml", "--project-name", "app",
		"--project-directory", "deploy", "--env-file", ".env.ci", "--profile", "ci", "--profile", "db",
		"up", "--wait",
	}))
	assert.Check(t, cmp.DeepEqual(Project{}.Args("ps"), []string{"compose", "ps"}))
}

func TestSplitList(t *testing.T) {
	cases := map[string][]string{
		"":                       nil,
		"a":                      {"a"},
		"a,b":                    {"a", "b"},
		" a , b ,,":              {"a", "b"},
		"a\nb\r\nc":              {"a", "b", "c"},
		"compose.yaml,\n ci.yml": {"compose.yaml", "ci.yml"},
	}
	for in, want := range cases {
		assert.Check(t, cmp.DeepEqual(SplitList(in), want), "input %q", in)
	}
}

func TestParsePS(t *testing.T) {
	db := Container{
		Name: "app-db-1", Service: "db", State: "running", Health: "healthy",
		Publishers: []Publisher{
			{URL: "0.0.0.0", TargetPort: 5432, PublishedPort: 60763, Protocol: "tcp"},
			{URL: "::", TargetPort: 5432, PublishedPort: 60763, Protocol: "tcp"},
		},
	}
	migrate := Container{Name: "app-migrate-1", Service: "migrate", State: "exited", ExitCode: 0}

	t.Run("one object per line", func(t *testing.T) {
		out := `{"Name":"app-db-1","Service":"db","State":"running","Health":"healthy","ExitCode":0,"Publishers":[{"URL":"0.0.0.0","TargetPort":5432,"PublishedPort":60763,"Protocol":"tcp"},{"URL":"::","TargetPort":5432,"PublishedPort":60763,"Protocol":"tcp"}]}
{"Name":"app-migrate-1","Service":"migrate","State":"exited","Health":"","ExitCode":0,"Publishers":null}
`
		got, err := ParsePS([]byte(out))
		assert.NilError(t, err)
		assert.Check(t, cmp.DeepEqual(got, []Container{db, migrate}))
	})

	t.Run("array", func(t *testing.T) {
		out := `[{"Name":"app-db-1","Service":"db","State":"running","Health":"healthy","ExitCode":0,"Publishers":[{"URL":"0.0.0.0","TargetPort":5432,"PublishedPort":60763,"Protocol":"tcp"},{"URL":"::","TargetPort":5432,"PublishedPort":60763,"Protocol":"tcp"}]},{"Name":"app-migrate-1","Service":"migrate","State":"exited","ExitCode":0}]`
		got, err := ParsePS([]byte(out))
		assert.NilError(t, err)
		assert.Check(t, cmp.DeepEqual(got, []Container{db, migrate}))
	})

	t.Run("empty", func(t *testing.T) {
		got, err := ParsePS([]byte("\n"))
		assert.NilError(t, err)
		assert.Check(t, cmp.Len(got, 0))
	})

	t.Run("garbage", func(t *testing.T) {
		_, err := ParsePS([]byte("{nope"))
		assert.Check(t, cmp.ErrorContains(err, "parse docker compose ps"))
	})
}
