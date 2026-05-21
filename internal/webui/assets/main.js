(function () {
  const views = ["projects", "routes", "history", "config"];
  const state = { history: [], routes: [], selectedEntry: null };

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

  function historyRow(entry, cells) {
    return `<tr data-history-id="${escapeHTML(entry.id)}">${cells.map((c) => `<td>${c}</td>`).join("")}</tr>`;
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
      `${escapeHTML(r.mode)} <button type="button" data-test-route-index="${escapeHTML(r.index)}" data-test-route-method="${escapeHTML(r.method)}">Test</button>`,
    ])).join("") : '<tr><td colspan="3">No routes loaded.</td></tr>';
    for (const btn of $("routes-body").querySelectorAll("[data-test-route-index]")) {
      btn.addEventListener("click", () => openRouteTester(Number(btn.dataset.testRouteIndex), btn.dataset.testRouteMethod));
    }
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
      return historyRow(e, [
        escapeHTML(ts),
        `<span class="method">${escapeHTML(e.method || "-")}</span>`,
        escapeHTML(e.path || "-"),
        status,
        escapeHTML(e.durationMs ? `${e.durationMs.toFixed(1)}ms` : "-"),
        escapeHTML(e.clientIp || "-"),
      ]);
    }).join("") : '<tr><td colspan="6">Waiting for requests...</td></tr>';
    for (const tr of $("history-body").querySelectorAll("[data-history-id]")) {
      tr.addEventListener("click", () => openHistoryDetail(tr.dataset.historyId));
    }
  }

  function formatBody(capture) {
    if (!capture) return "-";
    if (capture.binary) return "Binary body omitted";
    if (!capture.body) return (capture.omitted || []).join(", ") || "-";
    const text = String(capture.body);
    try {
      return JSON.stringify(JSON.parse(text), null, 2);
    } catch (_) {
      return text;
    }
  }

  function captureBlock(title, capture) {
    const headers = capture?.headers || {};
    const headerText = Object.keys(headers).length ? JSON.stringify(headers, null, 2) : "-";
    const suffix = capture?.truncated ? `\n\n(truncated at ${capture.bodySize || 0} bytes)` : "";
    return `
      <section class="detail-section">
        <h3>${escapeHTML(title)}</h3>
        <div class="detail-grid">
          <div>Content-Type</div><div>${escapeHTML(capture?.contentType || "-")}</div>
          <div>Body size</div><div>${escapeHTML(capture?.bodySize ?? "-")}</div>
        </div>
        <pre class="detail-code">${escapeHTML(headerText)}</pre>
        <pre class="detail-code">${escapeHTML(formatBody(capture) + suffix)}</pre>
      </section>`;
  }

  function renderHistoryDetail(entry) {
    $("history-detail-title").textContent = `${entry.method || "-"} ${entry.path || "-"}`;
    $("history-detail-body").innerHTML = `
      <section class="detail-section">
        <h3>Summary</h3>
        <div class="detail-grid">
          <div>Status</div><div>${escapeHTML(entry.status || "-")}</div>
          <div>Duration</div><div>${escapeHTML(entry.durationMs ? `${entry.durationMs.toFixed(1)}ms` : "-")}</div>
          <div>Client</div><div>${escapeHTML(entry.clientIp || "-")}</div>
        </div>
      </section>
      <section class="detail-section">
        <h3>Matched route</h3>
        <div class="detail-grid">
          <div>Mode</div><div>${escapeHTML(entry.routeMode || "-")}</div>
          <div>Route index</div><div>${escapeHTML(entry.routeIndex ?? "-")}</div>
          <div>Case index</div><div>${escapeHTML(entry.caseIndex ?? "-")}</div>
          <div>Source</div><div>${escapeHTML(entry.routeSource || "-")}</div>
          <div>Proxy target</div><div>${escapeHTML(entry.proxyTarget || "-")}</div>
        </div>
      </section>
      ${captureBlock("Request", entry.request)}
      ${captureBlock("Response", entry.response)}
    `;
  }

  async function openHistoryDetail(id) {
    const entry = await getJSON(`/__fakeserver/api/history/${id}`);
    state.selectedEntry = entry;
    $("history-detail-drawer").hidden = false;
    $("replay-result").hidden = true;
    renderHistoryDetail(entry);
  }

  function shellQuote(value) {
    return `'${String(value).replace(/'/g, `'\\''`)}'`;
  }

  function buildCurl(entry) {
    const parts = ["curl"];
    const method = entry.method || "GET";
    if (!["GET", "HEAD"].includes(method)) {
      parts.push("-X", method);
    }
    const headers = entry.request?.headers || {};
    for (const [key, value] of Object.entries(headers)) {
      if (value === "***") continue;
      parts.push("-H", shellQuote(`${key}: ${value}`));
    }
    if (entry.request?.body && !["GET", "HEAD"].includes(method)) {
      parts.push("--data-raw", shellQuote(entry.request.body));
    }
    parts.push(shellQuote(`${location.origin}${entry.path || "/"}`));
    return parts.join(" ");
  }

  const forbiddenHeaders = [
    "host",
    "connection",
    "content-length",
    "cookie",
    "origin",
    "referer",
  ];

  function replayableHeaders(headers) {
    const out = {};
    const omitted = [];
    for (const [key, value] of Object.entries(headers || {})) {
      const lower = key.toLowerCase();
      if (value === "***" || forbiddenHeaders.includes(lower) || lower.startsWith("sec-")) {
        omitted.push(key);
        continue;
      }
      out[key] = value;
    }
    return { headers: out, omitted };
  }

  async function readFetchResponse(resp) {
    const headers = {};
    resp.headers.forEach((value, key) => { headers[key] = value; });
    const body = await resp.text();
    return { status: resp.status, headers, body };
  }

  async function replayEntry(entry) {
    if (!entry || (entry.path || "").startsWith("/__fakeserver/")) {
      return { error: "admin requests are not replayed from the UI" };
    }
    const method = entry.method || "GET";
    const { headers, omitted } = replayableHeaders(entry.request?.headers || {});
    const init = { method, headers };
    if (!["GET", "HEAD"].includes(method)) {
      if (entry.request?.body) {
        init.body = entry.request.body;
      } else {
        omitted.push("body not captured");
      }
    }
    const resp = await fetch(entry.path || "/", init);
    const result = await readFetchResponse(resp);
    result.omitted = omitted;
    return result;
  }

  async function replaySelectedEntry() {
    if (!state.selectedEntry) return;
    const box = $("replay-result");
    box.hidden = false;
    box.textContent = "Sending replay...";
    try {
      const result = await replayEntry(state.selectedEntry);
      box.textContent = JSON.stringify(result, null, 2);
    } catch (err) {
      box.textContent = String(err.message || err);
    }
  }

  function openRouteTester(index, method) {
    const route = state.routes.find((r) => Number(r.index) === index && r.method === method) || state.routes.find((r) => Number(r.index) === index);
    if (!route) return;
    $("route-tester").hidden = false;
    const methodSelect = $("tester-method");
    const methods = route.method === "*" ? ["GET", "POST", "PUT", "PATCH", "DELETE"] : [route.method];
    methodSelect.innerHTML = methods.map((m) => `<option value="${escapeHTML(m)}">${escapeHTML(m)}</option>`).join("");
    $("tester-path").value = route.path || "/";
    $("tester-query").value = "";
    $("tester-headers").value = ["POST", "PUT", "PATCH"].includes(methodSelect.value) ? "Content-Type: application/json" : "";
    $("tester-body").value = ["POST", "PUT", "PATCH"].includes(methodSelect.value) ? "{}" : "";
    $("tester-response").textContent = `Ready: ${methodSelect.value} ${route.path}`;
    location.hash = "routes";
  }

  function parseHeaders(text) {
    const headers = {};
    for (const line of String(text || "").split(/\r?\n/)) {
      const trimmed = line.trim();
      if (!trimmed) continue;
      const idx = trimmed.indexOf(":");
      if (idx <= 0) continue;
      headers[trimmed.slice(0, idx).trim()] = trimmed.slice(idx + 1).trim();
    }
    return headers;
  }

  function buildTesterPath() {
    let path = $("tester-path").value || "/";
    const query = new URLSearchParams();
    for (const line of $("tester-query").value.split(/\r?\n/)) {
      const trimmed = line.trim();
      if (!trimmed) continue;
      const idx = trimmed.indexOf("=");
      if (idx < 0) {
        query.append(trimmed, "");
      } else {
        query.append(trimmed.slice(0, idx), trimmed.slice(idx + 1));
      }
    }
    const qs = query.toString();
    if (qs) path += (path.includes("?") ? "&" : "?") + qs;
    return path;
  }

  async function sendTesterRequest() {
    const method = $("tester-method").value || "GET";
    const init = { method, headers: parseHeaders($("tester-headers").value) };
    if (!["GET", "HEAD"].includes(method) && $("tester-body").value) {
      init.body = $("tester-body").value;
    }
    const started = performance.now();
    const resp = await fetch(buildTesterPath(), init);
    const result = await readFetchResponse(resp);
    result.durationMs = Math.round((performance.now() - started) * 10) / 10;
    $("tester-response").textContent = JSON.stringify(result, null, 2);
  }

  async function copySelectedCurl() {
    if (!state.selectedEntry) return;
    const text = buildCurl(state.selectedEntry);
    try {
      await navigator.clipboard.writeText(text);
    } catch (_) {
      const area = document.createElement("textarea");
      area.value = text;
      document.body.appendChild(area);
      area.select();
      document.execCommand("copy");
      area.remove();
    }
    $("copy-curl-button").textContent = "Copied";
    setTimeout(() => { $("copy-curl-button").textContent = "Copy curl"; }, 1200);
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
  $("history-detail-close").addEventListener("click", () => {
    $("history-detail-drawer").hidden = true;
  });
  $("copy-curl-button").addEventListener("click", copySelectedCurl);
  $("replay-button").addEventListener("click", replaySelectedEntry);
  $("tester-send").addEventListener("click", () => {
    sendTesterRequest().catch((err) => {
      $("tester-response").textContent = String(err.message || err);
    });
  });
  onHashChange();
  refreshAll();
  connectEvents();
}());
