# setup-compose

Starts a Docker Compose project for the rest of the job: brings its services up in
dependency order, waits until they are healthy and their ports answer, tells later steps
where each one is, keeps every service's logs if the job fails, and tears it all down.

```yaml
version: 2.1

functions:
  setup-compose: github.com/circleci-functions/setup-compose@v0.1.0

jobs:
  integration:
    machine:
      image: ubuntu-2404:current
    steps:
      - checkout
      - setup-compose:
          with:
            file: compose.yaml,compose.ci.yaml
            profile: ci
            wait-timeout: 10m
      - run: go test ./integration/... # connects to $COMPOSE_POSTGRES_HOST:$COMPOSE_POSTGRES_PORT
      - setup-compose/down

workflows:
  main:
    jobs:
      - integration
```

`setup-compose` on its own runs `up`. The other commands are `setup-compose/logs` and
`setup-compose/down`.

## What `up` waits for

`up` runs `docker compose up --detach --wait`, so services start in the order their
`depends_on` entries give, and `up` returns once every service is healthy, or running if it
has no healthcheck. Then:

- **Ports of services without a healthcheck.** A container can be running long before the
  server in it listens. `up` waits until each published TCP port answers, reading from the
  connection as well as opening it, since Docker accepts connections on a published port
  whether or not anything is listening behind it. Turn this off with `wait-ports: false`.
- **One-shot services.** A service that runs to completion, such as a migration, is done
  once it exits 0, even if no other service waits for it. One that exits with any other
  status fails the step.

Everything shares `wait-timeout`. If a service never comes up, the step fails and prints
the end of that service's logs.

## Reaching the services

For each service with a published port, `up` exports:

| Variable | Value |
|---|---|
| `COMPOSE_<SERVICE>_HOST` | The address to connect to, `127.0.0.1` for a port published on every interface |
| `COMPOSE_<SERVICE>_PORT` | The first published port, TCP before UDP, lowest container port first |
| `COMPOSE_<SERVICE>_PORT_<TARGET>` | The host port for each published container port, with `_UDP` after UDP ones |

`<SERVICE>` is the service name upper-cased, with anything but letters and digits replaced
by `_`: service `my-db` publishing `"5432"` gives `COMPOSE_MY_DB_HOST` and
`COMPOSE_MY_DB_PORT_5432`. The prefix keeps them apart from the variables your own code
reads, such as `REDIS_HOST`; map them in the step that needs them:

```yaml
      - run:
          name: Integration tests
          command: DATABASE_URL="postgres://app@$COMPOSE_POSTGRES_HOST:$COMPOSE_POSTGRES_PORT/app" make test
```

`up` also exports `COMPOSE_FILE`, `COMPOSE_PROJECT_NAME`, `COMPOSE_PROFILES` and
`COMPOSE_ENV_FILES` for the flags you set, so `docker compose` in a later step, and
`setup-compose/logs` and `setup-compose/down`, work on the same project without repeating
them. Pass `project-directory` to those again if you set it. Turn all exports off with
`export-env: false`.

## Tests inside the project, and remote Docker

Under `setup_remote_docker`, containers run on a separate machine, so their published ports
are out of the job's reach. `up` says so and exports no `COMPOSE_<SERVICE>_*` variables.
Put the tests in the project instead, reaching the other services by name, and run them
with `exit-code-from`:

```yaml
  integration:
    docker:
      - image: cimg/base:current
    steps:
      - checkout
      - setup_remote_docker
      - setup-compose:
          with:
            file: compose.test.yaml
            build: true
            exit-code-from: test
```

This runs `docker compose up --abort-on-container-exit --exit-code-from test` in the
foreground. The step passes or fails with the `test` service, and the others are stopped
when it exits. Waiting, exports and log collection are skipped, since every service's
output is already in the step's.

## Logs

`up` writes each service's logs to a file as they come, and if the job fails, stores them
as artifacts under `compose-logs/<project>/<service>.log`. They are kept from each
container's first line, including when `up` itself fails. Turn this off with
`logs-on-fail: false`.

To keep the logs whether or not the job fails, add a `logs` step. It stores them now, one
`<service>.log` each and `ps.txt`, under `destination`:

```yaml
      - setup-compose/logs:
          with:
            services: api,worker
            destination: logs/after-tests
```

## Tearing down

```yaml
      - setup-compose/down
```

runs `docker compose down --remove-orphans --volumes`. Pass `volumes: false` to keep named
volumes. A job's executor is thrown away when the job ends, so `down` matters most when
later steps need the ports or the disk space back, or the engine outlives the job.

## Installing Compose

`up` uses the Compose already installed, as it is on CircleCI's Linux machine images and on
`cimg/base`. If there is none, it installs the version pinned in this release. To choose
one, set `compose-version` to a version such as `v5.6.0`, or `latest`. Downloads are
checked against the checksums Docker publishes. The Docker CLI itself has to be there
already.

## Arguments

Every command takes these, to pick the project:

| Argument | Default | Description |
|---|---|---|
| `file` | | Compose files, comma or newline separated. Empty uses `COMPOSE_FILE`, or `compose.yaml` in the working directory. |
| `project-name` | | Project name. Empty uses `COMPOSE_PROJECT_NAME`, or the name of the project directory. |
| `project-directory` | | Directory relative paths in the compose files are resolved against. Empty is the first file's directory. |
| `env-file` | | Files of variables to interpolate into the compose files, comma or newline separated. Empty uses `.env` in the project directory. |
| `profile` | | Profiles to enable, comma or newline separated. |

### `up`

| Argument | Default | Description |
|---|---|---|
| `services` | | Services to start, comma or newline separated, with the services they depend on. Empty starts them all. |
| `build` | `false` | Build images before starting, even ones that already exist. |
| `pull` | | When to pull images: `always`, `missing`, `never` or `build`. Empty leaves it to compose. |
| `wait-timeout` | `"5m"` | How long to wait for every service to be ready, as a duration such as `90s` or `10m`. |
| `wait-ports` | `true` | Wait for the published ports of services without a healthcheck to answer. |
| `export-env` | `true` | Export each service's address and the project's settings for later steps. |
| `logs-on-fail` | `true` | Keep every service's logs, and store them as artifacts if the job fails. |
| `exit-code-from` | | Run in the foreground until this service exits, and exit with its status. |
| `compose-version` | | Compose version to install, or `latest`. Empty keeps the Compose already installed. |

### `logs`

| Argument | Default | Description |
|---|---|---|
| `services` | | Services whose logs to store, comma or newline separated. Empty stores them all. |
| `destination` | `compose-logs` | Artifact path to store the logs under. |

### `down`

| Argument | Default | Description |
|---|---|---|
| `volumes` | `true` | Remove the project's named volumes and its containers' anonymous ones. |
| `timeout` | `"10s"` | How long to give containers to stop before killing them. |

## Platform support

Linux only: machine executors, and Docker executors with `setup_remote_docker`. macOS
executors have no Docker engine, and Windows ones run Windows containers only.
