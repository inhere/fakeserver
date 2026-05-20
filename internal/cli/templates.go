package cli

// initTemplate is the JSON5 written by `fakeserver init`. Includes brief
// inline comments to onboard new users. Keep it small — heavy examples
// belong in docs/examples.
const initTemplate = `{
  // Fakeserver configuration — see docs/fakeserver-design.md for full schema.
  server: {
    port: 5090,
    cors: true,
  },

  // Routes are matched by rux's radix tree: static > param > wildcard.
  // The first matching route wins; multiple responses for the same
  // method+path go into a single route's "cases" array.
  routes: [
    { method: "GET", path: "/ping", body: "pong" },

    {
      method: "GET",
      path: "/users/{id}",
      body: {
        id: "{{ .request.params.id }}",
        name: "demo-user",
      },
    },
  ],
}
`

// initEnvTemplate is written by `fakeserver init --with-env`. The schema
// matches design §8.2.
const initEnvTemplate = `{
  "$default": {
    // Variables shared across every environment. Override per-env below.
  },
  dev: {
    host: "localhost:5090",
  },
  staging: {
    host: "stage.api.example.com",
  },
  prod: {
    host: "api.example.com",
  },
}
`
