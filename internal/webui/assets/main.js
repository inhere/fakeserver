(function () {
  const views = ["projects", "routes", "history", "config"];
  const state = { history: [], routes: [] };

  const $ = (id) => document.getElementById(id);

  function setStatus(text, live) {
    $("connection-state").textContent = text;
    $("connection-state").classList.toggle("live", live);
    $("server-status").textContent = live ? "stream online" : text;
  }

  function escapeHTML(value) {
    return String(value ?? "").replace(/[&<>"']/g, (ch) => ({
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      '"': "&quot;",
      "'": "&#39;",
    }[ch]));
  }

  function row(cells) {
    return `<tr>${cells.map((c) => `<td>${c}</td>`).join("")}</tr>`;
  }

  async function getJSON(path) {
    const resp = await fetch(path, { headers: { Accept: "application/json" } });
    if (!resp.ok) throw new Error(`${path}: ${resp.status}`);
    return resp.json();
  }

  function renderProjects(projects) {
    const body = $("projects-body");
    if (!projects.length) {
      body.innerHTML = '<tr><td colspan="5">No projects registered.</td></tr>';
      return;
    }
    body.innerHTML = projects.map((p) => row([
      escapeHTML(p.name || p.id || "-"),
      escapeHTML(p.status || (p.pidFile ? "registered" : "idle")),
      escapeHTML(p.lastEnv || "-"),
      escapeHTML(p.lastPort || "-"),
      escapeHTML(p.lastRunAt || "-"),
    ])).join("");
  }

  function renderRoutes(routes) {
    state.routes = routes;
    $("route-count").textContent = `${routes.length} routes`;
    $("routes-body").innerHTML = routes.length ? routes.map((r) => row([
      `<span class="method">${escapeHTML(r.method)}</span>`,
      escapeHTML(r.path),
      escapeHTML(r.mode),
    ])).join("") : '<tr><td colspan="3">No routes loaded.</td></tr>';
  }

  function statusClass(status) {
    if (status >= 500) return "status-bad";
    if (status >= 400) return "status-warn";
    return "";
  }

  function renderHistory() {
    $("history-count").textContent = `${state.history.length} events`;
    $("history-body").innerHTML = state.history.length ? state.history.slice(-200).reverse().map((e) => {
      const ts = e.ts ? new Date(e.ts).toLocaleTimeString() : "-";
      const status = `<span class="${statusClass(e.status)}">${escapeHTML(e.status || "-")}</span>`;
      return row([
        escapeHTML(ts),
        `<span class="method">${escapeHTML(e.method || "-")}</span>`,
        escapeHTML(e.path || "-"),
        status,
        escapeHTML(e.durationMs ? `${e.durationMs.toFixed(1)}ms` : "-"),
        escapeHTML(e.clientIp || "-"),
      ]);
    }).join("") : '<tr><td colspan="6">Waiting for requests...</td></tr>';
  }

  function renderConfig(cfg) {
    $("config-body").textContent = JSON.stringify(cfg, null, 2);
  }

  async function refreshAll() {
    try {
      const [projects, routes, history, cfg] = await Promise.all([
        getJSON("/__fakeserver/api/projects"),
        getJSON("/__fakeserver/routes"),
        getJSON("/__fakeserver/api/history"),
        getJSON("/__fakeserver/api/config"),
      ]);
      state.history = history;
      renderProjects(projects);
      renderRoutes(routes);
      renderHistory();
      renderConfig(cfg);
    } catch (err) {
      setStatus(err.message, false);
    }
  }

  function showView(name) {
    const active = views.includes(name) ? name : "history";
    for (const view of views) {
      $(`view-${view}`).hidden = view !== active;
      document.querySelector(`[data-view="${view}"]`).classList.toggle("active", view === active);
    }
    $("view-title").textContent = active[0].toUpperCase() + active.slice(1);
  }

  function onHashChange() {
    showView((location.hash || "#history").slice(1));
  }

  function showReload(diff) {
    const added = diff.added?.length || 0;
    const removed = diff.removed?.length || 0;
    const changed = diff.changed?.length || 0;
    const banner = $("reload-banner");
    banner.hidden = false;
    banner.textContent = `Config reloaded: ${added} added, ${removed} removed, ${changed} changed`;
    refreshAll();
  }

  function connectEvents() {
    if (!window.EventSource) {
      setStatus("EventSource unsupported", false);
      return;
    }
    const source = new EventSource("/__fakeserver/events");
    source.onopen = () => setStatus("live", true);
    source.onerror = () => setStatus("reconnecting", false);
    source.addEventListener("request", (event) => {
      state.history.push(JSON.parse(event.data));
      renderHistory();
    });
    source.addEventListener("reload", (event) => {
      showReload(JSON.parse(event.data));
    });
  }

  window.addEventListener("hashchange", onHashChange);
  onHashChange();
  refreshAll();
  connectEvents();
}());
