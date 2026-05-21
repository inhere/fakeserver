package cli

type generatedFile struct {
	Path string
	Body string
}

func fullInitFiles() []generatedFile {
	return []generatedFile{
		{Path: "fakeserver.json5", Body: fullConfigTemplate},
		{Path: "fakeserver.env.json5", Body: fullEnvTemplate},
		{Path: ".fakeserver/routes/health.json5", Body: fullHealthRoutesTemplate},
		{Path: ".fakeserver/routes/users.json5", Body: fullUsersRoutesTemplate},
		{Path: ".fakeserver/routes/orders.json5", Body: fullOrdersRoutesTemplate},
		{Path: ".fakeserver/routes/auth.json5", Body: fullAuthRoutesTemplate},
		{Path: ".fakeserver/routes/files.json5", Body: fullFilesRoutesTemplate},
		{Path: ".fakeserver/routes/proxy.json5", Body: fullProxyRoutesTemplate},
		{Path: ".fakeserver/fixtures/report.json", Body: fullReportFixture},
		{Path: ".fakeserver/fixtures/readme.txt", Body: fullReadmeFixture},
	}
}

const fullConfigTemplate = `{
  // Full fakeserver example for frontend development.
  // Start: fakeserver serve -c fakeserver.json5 --env dev
  // UI:    http://localhost:5090/__fakeserver/ui/
  server: {
    host: "127.0.0.1",
    port: 5090,
    cors: {
      origins: ["*"],
      methods: ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"],
      headers: ["Content-Type", "Authorization", "X-Request-Id", "X-Debug"],
      allowCredentials: false,
    },
    log: true,
    maxBodySize: "2MiB",
    adminEnabled: true,
    historySize: 200,
    osenvWhitelist: ["USER", "USERNAME", "COMPUTERNAME", "HOSTNAME", "FAKESERVER_DEMO_TOKEN"],
    fakerSeed: 42,
    projectName: "fakeserver-full-demo",
  },

  fallback: "echo",

  globals: {
    appName: "Lite Tools API Mock",
    apiVersion: "v1",
  },

  routes: [
    "@.fakeserver/routes/health.json5",
    "@.fakeserver/routes/users.json5",
    "@.fakeserver/routes/orders.json5",
    "@.fakeserver/routes/auth.json5",
    "@.fakeserver/routes/files.json5",
    "@.fakeserver/routes/proxy.json5",
  ],
}
`

const fullEnvTemplate = `{
  "$active": "dev",

  "$default": {
    tenant: "lite-tools",
    currency: "CNY",
    region: "cn-north",
    demoToken: "{{ osenv \"FAKESERVER_DEMO_TOKEN\" \"demo-token-local\" }}",
    requestPrefix: "fs",
  },

  dev: {
    baseUrl: "http://localhost:5090",
    upstreamBase: "https://httpbin.org",
    databaseName: "fakeserver_dev",
  },

  staging: {
    baseUrl: "https://staging.example.test",
    upstreamBase: "https://httpbin.org",
    databaseName: "fakeserver_staging",
  },
}
`

const fullHealthRoutesTemplate = `[
  {
    method: "GET",
    path: "/ping",
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      "X-Request-Id": "{{ .env.requestPrefix }}-{{ shortid }}",
    },
    body: "pong",
  },
  {
    method: "GET",
    path: "/api/meta",
    headers: {
      "Content-Type": "application/json; charset=utf-8",
      "X-Request-Id": "{{ .env.requestPrefix }}-{{ shortid }}",
    },
    body: {
      app: "Lite Tools API Mock",
      version: "v1",
      envBaseUrl: "{{ .env.baseUrl }}",
      tenant: "{{ .env.tenant }}",
      region: "{{ .env.region }}",
      now: "{{ now \"2006-01-02T15:04:05Z07:00\" }}",
      request: {
        method: "{{ .request.method }}",
        path: "{{ .request.path }}",
        ip: "{{ .request.ip }}",
        userAgent: "{{ index .request.headers \"User-Agent\" }}",
      },
      osUser: "{{ osenv \"USERNAME\" }}{{ osenv \"USER\" }}",
      features: {
        checkout: true,
        betaSearch: true,
        auditLog: true,
      },
    },
  },
]
`

const fullUsersRoutesTemplate = `[
  {
    method: "GET",
    path: "/api/users",
    delay: "30ms",
    headers: {
      "Content-Type": "application/json; charset=utf-8",
      "X-Total-Count": "3",
      "X-Request-Id": "{{ .env.requestPrefix }}-{{ shortid }}",
    },
    body: {
      page: "{{ default \"1\" .request.query.page }}",
      keyword: "{{ default \"\" .request.query.q }}",
      items: [
        { id: "u-1001", name: "{{ fakeName }}", email: "{{ fakeEmail }}", role: "admin", city: "{{ fakeCity }}", active: true },
        { id: "u-1002", name: "{{ fakeName }}", email: "{{ fakeEmail }}", role: "editor", city: "{{ fakeCity }}", active: true },
        { id: "u-1003", name: "{{ fakeName }}", email: "{{ fakeEmail }}", role: "viewer", city: "{{ fakeCity }}", active: false },
      ],
    },
  },
  {
    method: "GET",
    path: "/api/users/{id}",
    headers: {
      "Content-Type": "application/json; charset=utf-8",
      "X-Request-Id": "{{ .env.requestPrefix }}-{{ shortid }}",
    },
    body: {
      id: "{{ .request.params.id }}",
      name: "{{ fakeName }}",
      username: "{{ fakeUsername }}",
      email: "{{ fakeEmail }}",
      phone: "{{ fakePhone }}",
      company: "{{ fakeCompany }}",
      job: "{{ fakeJob }}",
      profileUrl: "{{ .env.baseUrl }}/api/users/{{ .request.params.id }}",
      createdAt: "{{ fakePastDate }}",
    },
  },
  {
    method: "POST",
    path: "/api/users",
    strategy: "first-match",
    cases: [
      {
        when: "request.body.name == \"\"",
        status: 400,
        headers: { "Content-Type": "application/json; charset=utf-8" },
        body: { error: "validation_failed", message: "name is required" },
      },
      {
        when: "request.query.fail == \"1\"",
        status: 500,
        delay: "120ms",
        headers: { "Content-Type": "application/json; charset=utf-8" },
        body: { error: "simulated_failure", message: "query fail=1 forces a 500 response" },
      },
      {
        status: 201,
        headers: {
          "Content-Type": "application/json; charset=utf-8",
          "Location": "/api/users/{{ shortid }}",
        },
        body: {
          id: "{{ shortid }}",
          name: "Created User",
          role: "viewer",
          createdAt: "{{ now \"2006-01-02T15:04:05Z07:00\" }}",
        },
      },
    ],
  },
]
`

const fullOrdersRoutesTemplate = `[
  {
    method: "GET",
    path: "/api/orders",
    strategy: "weighted",
    cases: [
      {
        weight: 8,
        status: 200,
        headers: { "Content-Type": "application/json; charset=utf-8" },
        body: {
          currency: "{{ .env.currency }}",
          items: [
            { id: "ord-{{ randString 6 \"hex\" }}", customer: "{{ fakeName }}", amount: "{{ fakeFloatRange 100 900 }}", status: "paid" },
            { id: "ord-{{ randString 6 \"hex\" }}", customer: "{{ fakeName }}", amount: "{{ fakeFloatRange 20 300 }}", status: "pending" },
          ],
        },
      },
      {
        weight: 2,
        status: 503,
        delay: "200ms",
        headers: { "Content-Type": "application/json; charset=utf-8" },
        body: { error: "temporary_unavailable", retryAfterSeconds: "{{ randInt 3 10 }}" },
      },
    ],
  },
  {
    method: "GET",
    path: "/api/orders/{id}",
    headers: { "Content-Type": "application/json; charset=utf-8" },
    body: {
      id: "{{ .request.params.id }}",
      traceId: "{{ uuid }}",
      customer: { name: "{{ fakeName }}", email: "{{ fakeEmail }}" },
      shipping: { city: "{{ fakeCity }}", address: "{{ fakeAddress }}" },
      lines: [
        { sku: "SKU-100", name: "{{ title (fakeWord) }}", quantity: "{{ randInt 1 3 }}", price: "{{ fakeFloatRange 9 99 }}" },
        { sku: "SKU-200", name: "{{ title (fakeWord) }}", quantity: "{{ randInt 1 2 }}", price: "{{ fakeFloatRange 20 199 }}" },
      ],
    },
  },
]
`

const fullAuthRoutesTemplate = `[
  {
    method: "POST",
    path: "/api/auth/login",
    strategy: "first-match",
    cases: [
      {
        when: "request.body.username == \"locked\"",
        status: 423,
        headers: { "Content-Type": "application/json; charset=utf-8" },
        body: { error: "account_locked", message: "This demo account is locked." },
      },
      {
        when: "request.body.password == \"bad\"",
        status: 401,
        headers: { "Content-Type": "application/json; charset=utf-8" },
        body: { error: "invalid_credentials", message: "Use any password except 'bad'." },
      },
      {
        status: 200,
        headers: { "Content-Type": "application/json; charset=utf-8" },
        body: {
          accessToken: "demo.{{ b64enc .env.demoToken }}.{{ randString 16 \"base62\" }}",
          tokenType: "Bearer",
          expiresIn: 3600,
          user: { id: "u-login", name: "Demo User", email: "demo@example.test" },
        },
      },
    ],
  },
]
`

const fullFilesRoutesTemplate = `[
  {
    method: "GET",
    path: "/api/report",
    headers: { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store" },
    bodyFile: "../fixtures/report.json",
  },
  {
    method: "GET",
    path: "/api/readme",
    headers: { "Content-Type": "text/plain; charset=utf-8" },
    bodyFile: "../fixtures/readme.txt",
  },
]
`

const fullProxyRoutesTemplate = `[
  {
    method: ["GET", "POST"],
    path: "/proxy/httpbin/*rest",
    proxy: {
      target: "https://httpbin.org",
      stripPathPrefix: "/proxy/httpbin",
      timeout: "10s",
      headers: {
        "X-Fakeserver-Project": "{{ .env.tenant }}",
        "X-Fakeserver-Trace": "{{ uuid }}",
      },
      responseHeaders: {
        "X-Served-By": "fakeserver-proxy",
      },
    },
  },
]
`

const fullReportFixture = `{
  "reportId": "rpt-demo-001",
  "title": "Daily Lite Tools Report",
  "status": "ready",
  "totals": {
    "requests": 128,
    "errors": 3,
    "latencyP95Ms": 42
  },
  "sections": [
    { "name": "mock", "enabled": true },
    { "name": "proxy", "enabled": true },
    { "name": "webui", "enabled": true }
  ]
}
`

const fullReadmeFixture = `fakeserver demo fixture

This response is served from .fakeserver/fixtures/readme.txt via bodyFile.
Use GET /api/report for a JSON bodyFile example.
`
