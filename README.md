# fakeserver

English | [简体中文](README.zh-CN.md)

`fakeserver` is a configuration-driven HTTP mock/fake server for frontend integration, failure-state simulation, local substitutes for unavailable services, and request inspection.

It is distributed as a single Go binary. The current implementation provides:

- An httpbin-style echo service when no route matches.
- JSON5 configuration with mock routes, status codes, headers, delays, response bodies, and `bodyFile` fixtures.
- Go templates with request context, environment variables, random/time/UUID/Faker helpers, and type-preserving `jsonValue` values.
- `cases`, matching strategies, and named scenarios for switching business states.
- Per-route reverse proxying, so mocked and real APIs can run together.
- Hot reload with filesystem notifications and polling fallback.
- A built-in Web UI, request history ring buffer, route replay, scenario controls, and request/reload SSE events.
- Project registration through `list` and `use`, plus `check` and `doctor` diagnostics.

## Installation

For local development:

```bash
go run ./cmd/fakeserver --help
```

The recommended installation uses the Makefile. It embeds the version, commit, and build time; `upx` is used only when available:

```bash
make install
```

Alternatively install with Go:

```bash
go install ./cmd/fakeserver
```

Without Makefile ldflags, version metadata is obtained from Go build information and the current Git checkout when available.

Build the current platform binary with either:

```bash
go build -o fakeserver ./cmd/fakeserver
make build
```

## Quick start

Generate a complete frontend-oriented example in the project directory:

```bash
fakeserver init --full
```

This creates `fakeserver.json5`, `fakeserver.env.json5`, and a `.fakeserver/` directory containing route and fixture examples.

Validate and diagnose the project:

```bash
fakeserver doctor -c fakeserver.json5 --env dev
fakeserver check --strict -c fakeserver.json5
```

Start the server:

```bash
fakeserver serve -c fakeserver.json5 --env dev
```

The default port is `5090`. When the admin UI is enabled, open `http://127.0.0.1:5090/__fakeserver/ui/`. Example requests:

```bash
curl http://127.0.0.1:5090/ping
curl http://127.0.0.1:5090/api/users
curl "http://127.0.0.1:5090/api/users?empty=1"
```

## Commands

```text
check    Load and validate a config; --strict also checks common template risks.
doctor   Diagnose config, env/include/bodyFile, port, proxy, and admin exposure risks.
init     Generate starter files; supports --with-env, --full, and --force.
list     List registered projects and their running/idle status.
routes   Print a route summary without starting the server.
serve    Start the HTTP server.
use      Mark a registered project as the most recently used project.
```

Useful `serve` options:

- `-c, --config`: comma-separated config paths; when omitted, the current directory is searched.
- `-e, --env`: environment segment from `fakeserver.env.json5`.
- `-p, --port` and `--host`: override `server.port` and `server.host` (defaults are `5090` and `0.0.0.0`). Restart after changing the effective listen address.
- `--var key=value`: repeatable environment-variable override; comma-separated values are accepted.
- `--scenario`: default scenario for this process.
- `--no-watch`: disable hot reload (the default watcher combines fsnotify with one-second polling).
- `--no-cors`: disable CORS middleware.
- `-q, --quiet`: suppress request access logs.

## Configuration

Minimal example:

```json5
{
  server: { host: "127.0.0.1", port: 5090, cors: true },
  routes: [
    { method: "GET", path: "/ping", body: "pong" },
    {
      method: "GET",
      path: "/users/{id}",
      headers: { "Content-Type": "application/json; charset=utf-8" },
      body: {
        id: "{{ .request.params.id }}",
        name: "{{ fakeName }}",
        requestId: "{{ uuid }}",
      },
    },
  ],
}
```

Routes may be declared inline or included with `@path/to/file.json5`; include paths are relative to the containing config file. `bodyFile` paths are relative to the route file.

Top-level fields include:

- `server`: listen settings, CORS and access logging, request-body limits, admin/UI, history/capture, and the default scenario.
- `globals`: values available to templates.
- `routes`: mock and proxy route declarations.
- `scenarios`: named mappings from `METHOD /path` to a route case.
- `fallback`: behavior when no route matches: `echo` (the default), `404`, or an object with `status`, `headers`, and `body`/`bodyFile` (status defaults to 404). Every fallback response has an `X-Fakeserver-Fallback` header.

For an internal-service substitute, return a consistent error object:

```json5
fallback: {
  status: 404,
  body: {
    data: null,
    status: 404,
    code: 404,
    message: "fakeserver: no route for {{ .request.method }} {{ .request.path }}",
  },
}
```

## Cases, scenarios, and proxy routes

Cases select different responses on the same route:

```json5
{
  method: "GET", path: "/api/users", strategy: "first-match",
  cases: [
    { name: "empty", when: "request.query.empty == \"1\"", status: 200, body: { items: [] } },
    { name: "server-error", when: "request.query.fail == \"1\"", status: 500, body: { error: "failed" } },
    { name: "success", status: 200, body: { items: [{ id: "u-1", name: "{{ fakeName }}" }] } },
  ],
}
```

Scenario precedence is `X-Fakeserver-Scenario` header, Web UI selection, `--scenario`, `server.scenario`, then route strategy. A single request can override it:

```bash
curl -H "X-Fakeserver-Scenario: serverErrors" http://127.0.0.1:5090/api/users
```

Proxy routes forward selected methods to a real backend. `proxy` is mutually exclusive with mock response fields such as `body`, `bodyFile`, `cases`, `status`, `headers`, and `delay`.

```json5
{
  method: ["GET", "POST"], path: "/proxy/httpbin/*rest",
  proxy: {
    target: "https://httpbin.org",
    stripPathPrefix: "/proxy/httpbin",
    timeout: "10s",
    headers: { "X-Fakeserver-Trace": "{{ uuid }}" },
    responseHeaders: { "X-Served-By": "fakeserver-proxy" },
  },
}
```

## Environment files and templates

`fakeserver.env.json5` holds named environment segments. Selection precedence is `--env`, `FAKESERVER_ENV`, `$active`, then the first non-`$default` segment. Templates access values through `.env`; operating-system variables are available through `.osenv`.

Useful context fields are `.request.method`, `.request.path`, `.request.params`, `.request.query`, `.request.headers`, `.request.body`, `.env`, `.osenv`, and `.config`. Header names containing hyphens use `index`, for example `{{ index .request.headers "User-Agent" }}`.

Common helpers include `uuid`, `shortid`, `now`, `randInt`, `randString`, `fakeName`, `fakeEmail`, `fakeCity`, and `fake`.

In JSON bodies, `jsonValue` preserves the original JSON type when the field consists of one complete action, for example `{{ jsonValue .request.body.id }}` keeps a numeric ID numeric. Mixed text remains a string; headers render JSON text. `toJson` and `fromJson` produce strings in normal template rendering.

## Web UI, admin endpoints, and security

When `server.adminEnabled` is true, fakeserver mounts:

```text
/__fakeserver/ui/
/__fakeserver/healthz
/__fakeserver/routes
/__fakeserver/events
/__fakeserver/api/projects
/__fakeserver/api/config
/__fakeserver/api/history
/__fakeserver/api/history/{id}
/__fakeserver/api/scenario
```

Admin and UI requests are restricted to loopback clients by default. `GET /__fakeserver/healthz` remains reachable from any source. To open the UI from another machine or from a container through a port mapping, set `server.adminAllowRemote: true`. If the server listens on a non-loopback address and remote admin access is enabled, fakeserver prints a warning. Keep `adminEnabled` disabled or bind to `127.0.0.1` for local-only use.

The UI displays routes, config, projects, and request history; shows matched route/case and proxy details; copies and replays curl requests; tests routes; and changes the selected scenario or per-route case override.

## Development

```bash
go test ./...
go vet ./...
go build -o fakeserver ./cmd/fakeserver
```

`make check` runs formatting validation, `go vet`, and tests. Main packages are `internal/cli`, `internal/config`, `internal/mock`, `internal/proxy`, `internal/tpl`, `internal/middleware`, `internal/recorder`, and `internal/webui`. See `docs/usage/frontend-workflow.md` for a frontend integration workflow.
