(() => {
  "use strict";

  // ---------------------------------------------------------------- vocab
  const PRIO = {
    P0: "Блокирует безопасный запуск или грозит потерей денег",
    P1: "До первых серьёзных платящих клиентов (до 100)",
    P2: "Рост и удержание (до 1 000)",
    P3: "Можно отложить",
  };
  const STATUS = {
    todo:    { label: "Не начато",     color: "var(--dim)" },
    doing:   { label: "В работе",      color: "var(--accent)" },
    blocked: { label: "Заблокировано", color: "var(--warn)" },
    done:    { label: "Сделано",       color: "var(--ok)" },
    wontfix: { label: "Не делаем",     color: "var(--idle)" },
  };
  const SEV = {
    critical: { label: "Critical", color: "var(--crit)", rank: 0 },
    high:     { label: "High",     color: "var(--bad)",  rank: 1 },
    medium:   { label: "Medium",   color: "var(--warn)", rank: 2 },
    low:      { label: "Low",      color: "var(--info)", rank: 3 },
  };
  const KIND = { security: "Безопасность", bug: "Баг", data: "Точность данных", ux: "UX / онбординг", gap: "Нет функции", ops: "Инфраструктура", risk: "Риск", business: "Бизнес" };
  const FEAT = { PASS: "var(--ok)", PARTIAL: "var(--warn)", FAIL: "var(--bad)", NOT_IMPLEMENTED: "var(--idle)", NOT_VERIFIED: "var(--info)" };
  const FEAT_LABEL = { PASS: "PASS", PARTIAL: "PARTIAL", FAIL: "FAIL", NOT_IMPLEMENTED: "NOT IMPLEMENTED", NOT_VERIFIED: "NOT VERIFIED" };
  const FIELD_LABEL = { status: "статус", priority: "приоритет", severity: "серьёзность", title: "название", fix: "что сделать", area: "область" };

  // ---------------------------------------------------------------- utils
  const $ = (id) => document.getElementById(id);
  const SVGNS = "http://www.w3.org/2000/svg";
  const el = (tag, attrs = {}, ...kids) => {
    const n = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) {
      if (v == null || v === false) continue;
      if (k === "class") n.className = v;
      else if (k === "style") n.setAttribute("style", v);
      else if (k.startsWith("on")) n.addEventListener(k.slice(2), v);
      else n.setAttribute(k, v === true ? "" : v);
    }
    for (const c of kids.flat()) if (c != null) n.append(c);
    return n;
  };
  const sv = (tag, attrs = {}, ...kids) => {
    const n = document.createElementNS(SVGNS, tag);
    for (const [k, v] of Object.entries(attrs)) if (v != null) n.setAttribute(k, v);
    for (const c of kids.flat()) if (c != null) n.append(c);
    return n;
  };
  const fmtDate = (iso) => { try { return new Date(iso).toLocaleString("ru-RU", { day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" }); } catch { return iso || ""; } };
  const fmtDay = (ymd) => { const [y, m, d] = ymd.split("-"); return `${d}.${m}`; };
  const fmtDayLong = (ymd) => { try { return new Date(ymd + "T12:00:00").toLocaleDateString("ru-RU", { day: "numeric", month: "long" }); } catch { return ymd; } };
  const ago = (iso) => {
    const s = (Date.now() - new Date(iso).getTime()) / 1000;
    if (s < 60) return "только что";
    if (s < 3600) return `${Math.floor(s / 60)} мин назад`;
    if (s < 86400) return `${Math.floor(s / 3600)} ч назад`;
    if (s < 7 * 86400) return `${Math.floor(s / 86400)} дн назад`;
    return fmtDate(iso);
  };
  const valLabel = (field, v) => field === "status" ? (STATUS[v]?.label || v) : field === "severity" ? (SEV[v]?.label || v) : v;

  async function api(method, path, body) {
    const res = await fetch(path, { method, headers: body ? { "Content-Type": "application/json" } : {}, body: body ? JSON.stringify(body) : undefined });
    if (!res.ok) {
      let msg = `Ошибка ${res.status}`;
      try { msg = (await res.json()).error || msg; } catch {}
      throw new Error(msg);
    }
    return res.status === 204 ? null : res.json();
  }

  // ---------------------------------------------------------------- state
  let items = [], features = [], stats = null;
  let itemsLoaded = false, featuresLoaded = false;
  const open = new Set();
  const history = new Map();
  const tableView = new Set();
  const ui = { q: "", prio: "all", status: "all", kind: "all", hideDone: false, days: 30 };
  try { Object.assign(ui, JSON.parse(localStorage.getItem("flox-devpanel-ui") || "{}")); } catch {}
  const saveUi = () => { try { localStorage.setItem("flox-devpanel-ui", JSON.stringify(ui)); } catch {} };

  function notice(msg, err) { const n = $("notice"); if (!msg) { n.hidden = true; return; } n.hidden = false; n.className = "notice" + (err ? " err" : ""); n.textContent = msg; }
  function sync(state, text) { $("syncDot").className = "dot " + state; $("syncText").textContent = text; }

  // ---------------------------------------------------------------- tabs
  const TABS = ["dashboard", "tasks", "features"];
  function currentTab() { const h = location.hash.slice(1); return TABS.includes(h) ? h : "dashboard"; }
  function showTab(t) {
    for (const name of TABS) {
      $("tab-" + name).setAttribute("aria-selected", String(name === t));
      const p = $("panel-" + name);
      p.hidden = name !== t;
      if (name === "tasks") p.style.display = name === t ? "grid" : "none";
    }
    if (t === "dashboard") renderDashboard();
  }
  for (const name of TABS) $("tab-" + name).onclick = () => { location.hash = name; };
  window.addEventListener("hashchange", () => showTab(currentTab()));

  function openItem(id) {
    ui.q = ""; ui.prio = "all"; ui.status = "all"; ui.kind = "all"; ui.hideDone = false; saveUi(); syncFilters();
    open.add(id); loadHistory(id);
    location.hash = "tasks";
    render();
    requestAnimationFrame(() => document.querySelector(`[data-id="${CSS.escape(id)}"]`)?.scrollIntoView({ block: "center", behavior: "smooth" }));
  }

  // ---------------------------------------------------------------- filters
  const prioChips = $("prioChips");
  for (const p of ["all", "P0", "P1", "P2", "P3"]) {
    prioChips.append(el("button", { class: "chip", type: "button", "data-p": p, onclick: () => { ui.prio = p; saveUi(); syncFilters(); render(); } }, p === "all" ? "Все" : p));
  }
  const fill = (sel, map, all) => { if (all) sel.append(el("option", { value: "all" }, all)); for (const [k, v] of Object.entries(map)) sel.append(el("option", { value: k }, typeof v === "string" ? v : v.label)); };
  fill($("fStatus"), STATUS, "Любой статус"); fill($("fKind"), KIND, "Любой тип");
  fill($("aPrio"), Object.fromEntries(Object.keys(PRIO).map(p => [p, p]))); fill($("aSev"), SEV); fill($("aKind"), KIND);
  const resetAdd = () => { $("aPrio").value = "P1"; $("aSev").value = "medium"; $("aKind").value = "bug"; };
  resetAdd();
  function syncFilters() {
    for (const b of prioChips.children) b.setAttribute("aria-pressed", String(b.dataset.p === ui.prio));
    $("fStatus").value = ui.status; $("fKind").value = ui.kind; $("q").value = ui.q; $("hideDone").checked = ui.hideDone;
  }
  syncFilters();
  $("fStatus").onchange = (e) => { ui.status = e.target.value; saveUi(); render(); };
  $("fKind").onchange = (e) => { ui.kind = e.target.value; saveUi(); render(); };
  $("hideDone").onchange = (e) => { ui.hideDone = e.target.checked; saveUi(); render(); };
  let qt; $("q").oninput = (e) => { clearTimeout(qt); qt = setTimeout(() => { ui.q = e.target.value; saveUi(); render(); }, 120); };
  $("days").value = String(ui.days);
  $("days").onchange = (e) => { ui.days = Number(e.target.value); saveUi(); loadStats(); };
  document.querySelectorAll("[data-table]").forEach(b => b.addEventListener("click", () => {
    const k = b.dataset.table;
    tableView.has(k) ? tableView.delete(k) : tableView.add(k);
    b.textContent = tableView.has(k) ? "График" : "Таблица";
    renderDashboard();
  }));

  // ---------------------------------------------------------------- writes
  async function patchItem(i, patch) {
    try {
      const saved = await api("PATCH", "/api/items/" + encodeURIComponent(i.id), patch);
      Object.assign(i, saved); history.delete(i.id); notice("");
      sync("live", "Сохранено " + fmtDate(saved.updatedAt));
      if (open.has(i.id)) loadHistory(i.id);
      loadStats();
    } catch (e) { notice("Не удалось сохранить: " + e.message, true); await load(); }
    render();
  }

  // ---------------------------------------------------------------- summary (gate + meters)
  function renderSummary() {
    const live = items.filter(i => i.status !== "wontfix");
    const by = (arr, s) => arr.filter(i => i.status === s).length;
    const total = live.length;
    $("totalTxt").textContent = total ? `${by(live, "done")} / ${total} сделано` : "—";
    const bar = $("totalBar"); bar.replaceChildren();
    if (total) for (const s of ["done", "doing", "blocked"]) bar.append(el("i", { class: s, style: `width:${by(live, s) / total * 100}%` }));
    const meters = $("meters");
    meters.querySelectorAll(".meter").forEach(m => m.remove());
    for (const p of Object.keys(PRIO)) {
      const g = live.filter(i => i.priority === p);
      const b = el("div", { class: "bar" });
      if (g.length) for (const s of ["done", "doing", "blocked"]) b.append(el("i", { class: s, style: `width:${by(g, s) / g.length * 100}%` }));
      meters.insertBefore(el("div", { class: "meter" }, el("div", { class: "lbl" }, el("span", {}, p), el("b", {}, `${by(g, "done")}/${g.length}`)), b), meters.querySelector(".overall"));
    }
    if (!itemsLoaded) return;
    const gate = $("gate");
    const p0open = live.filter(i => i.priority === "P0" && i.status !== "done");
    if (!total) { $("gateState").textContent = "Нет задач"; $("gateText").textContent = "Добавьте первую задачу на вкладке «Задачи»."; return; }
    gate.classList.toggle("ready", p0open.length === 0);
    if (p0open.length) {
      $("gateState").textContent = "Не готов к платным клиентам";
      const crit = p0open.filter(i => i.severity === "critical").length;
      $("gateText").textContent = `Открыто задач P0: ${p0open.length}` + (crit ? `, из них критических: ${crit}` : "") + ". Пока они не закрыты, брать деньги с клиентов рискованно.";
    } else {
      $("gateState").textContent = "P0 закрыт — можно идти в закрытую бету";
      $("gateText").textContent = "Следующий рубеж — P1: биллинг, онбординг и интеграции до первых 100 клиентов.";
    }
  }

  // ---------------------------------------------------------------- dashboard
  function renderDashboard() {
    renderSummary();
    if (!stats || $("panel-dashboard").hidden) return;
    const t = stats.totals;
    $("kOpen").textContent = t.open;
    $("kOpenS").textContent = `из ${t.all - t.wontfix}, сделано ${t.done}`;
    $("kP0").textContent = t.p0Open;
    $("kP0Box").classList.toggle("alert", t.p0Open > 0);
    $("kDone7").textContent = t.doneLast7;
    const wk = stats.weekly.slice(-5, -1).map(w => w.done);
    const avg = wk.length ? wk.reduce((a, b) => a + b, 0) / wk.length : 0;
    $("kDoneS").textContent = `в среднем ${avg.toFixed(1).replace(".", ",")} за неделю`;
    $("kDoing").textContent = t.doing;
    $("kBlocked").textContent = t.blocked ? `заблокировано: ${t.blocked}` : "ничего не заблокировано";

    drawBurn();
    drawWeekly();
    drawHBars($("cPrio"), stats.byPriority.map(p => ({ name: p.priority, value: p.open, note: `${p.done}/${p.total}` })), "Нет открытых задач.");
    drawHBars($("cArea"), stats.byKind.map(k => ({ name: KIND[k.kind] || k.kind, value: k.open })), "Нет открытых задач.");
    drawNextUp();
    drawFeed();
  }

  // Nice upper bound and ticks for a 0-based axis.
  function niceTicks(max, n = 4) {
    if (max <= 0) return [0, 1];
    const raw = max / n;
    const mag = Math.pow(10, Math.floor(Math.log10(raw)));
    const step = [1, 2, 5, 10].map(m => m * mag).find(s => s >= raw) || raw;
    const top = Math.ceil(max / step) * step;
    const out = [];
    for (let v = 0; v <= top + 1e-9; v += step) out.push(Math.round(v));
    return out;
  }

  function placeTip(box, tip, x, y) {
    const w = box.clientWidth;
    tip.style.left = Math.min(Math.max(x + 12, 0), Math.max(w - tip.offsetWidth, 0)) + "px";
    tip.style.top = Math.max(y - tip.offsetHeight - 10, 0) + "px";
  }

  function chartTable(box, head, rows) {
    box.replaceChildren(el("div", { class: "ctwrap" }, el("table", { class: "ctable" },
      el("thead", {}, el("tr", {}, head.map((h, i) => el("th", { class: i ? "n" : null }, h)))),
      el("tbody", {}, rows.map(r => el("tr", {}, r.map((c, i) => el("td", { class: i ? "n" : null }, String(c)))))))));
  }

  function drawBurn() {
    const box = $("cBurn");
    const data = stats.series;
    if (tableView.has("burn")) {
      chartTable(box, ["День", "Открыто", "P0 открыто", "Сделано"], [...data].reverse().map(p => [fmtDayLong(p.day), p.open, p.p0Open, p.done]));
      return;
    }
    if (!data.length) { box.replaceChildren(el("div", { class: "empty" }, "Истории пока нет.")); return; }
    const W = Math.max(box.clientWidth, 300), H = 220;
    const m = { l: 34, r: 74, t: 12, b: 26 };
    const iw = W - m.l - m.r, ih = H - m.t - m.b;
    const ticks = niceTicks(Math.max(...data.map(p => p.open), 1));
    const ymax = ticks[ticks.length - 1];
    const x = (i) => m.l + (data.length === 1 ? iw / 2 : (i / (data.length - 1)) * iw);
    const y = (v) => m.t + ih - (v / ymax) * ih;
    const svg = sv("svg", { viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": `Открытые задачи за ${data.length} дней: сейчас ${data[data.length - 1].open}, из них P0 ${data[data.length - 1].p0Open}` });
    for (const v of ticks) {
      svg.append(sv("line", { class: v === 0 ? "axis" : "gridl", x1: m.l, x2: m.l + iw, y1: y(v), y2: y(v) }));
      svg.append(sv("text", { x: m.l - 8, y: y(v) + 4, "text-anchor": "end" }, String(v)));
    }
    const every = Math.ceil(data.length / Math.max(2, Math.floor(iw / 64)));
    data.forEach((p, i) => {
      if ((data.length - 1 - i) % every === 0) svg.append(sv("text", { x: x(i), y: H - 6, "text-anchor": "middle" }, fmtDay(p.day)));
    });
    const path = (key) => data.map((p, i) => `${i ? "L" : "M"}${x(i).toFixed(1)},${y(p[key]).toFixed(1)}`).join("");
    svg.append(sv("path", { d: path("open"), fill: "none", stroke: "var(--s1)", "stroke-width": 2, "stroke-linejoin": "round", "stroke-linecap": "round" }));
    svg.append(sv("path", { d: path("p0Open"), fill: "none", stroke: "var(--s2)", "stroke-width": 2, "stroke-dasharray": "5 4", "stroke-linejoin": "round", "stroke-linecap": "round" }));
    // End markers + direct labels (text in ink, the dot carries identity).
    const last = data.length - 1, lp = data[last];
    let yo = y(lp.open), yp = y(lp.p0Open);
    if (Math.abs(yo - yp) < 14) { if (yo <= yp) { yo -= 7; yp += 7; } else { yo += 7; yp -= 7; } }
    for (const [v, ly, c, name] of [[lp.open, yo, "var(--s1)", "всего"], [lp.p0Open, yp, "var(--s2)", "P0"]]) {
      svg.append(sv("circle", { cx: x(last), cy: y(v), r: 4, fill: c, stroke: "var(--panel)", "stroke-width": 2 }));
      svg.append(sv("text", { class: "lbl", x: x(last) + 10, y: ly + 4 }, `${v} ${name}`));
    }
    const cross = sv("line", { class: "cross", y1: m.t, y2: m.t + ih, x1: -10, x2: -10, visibility: "hidden" });
    const d1 = sv("circle", { r: 4, fill: "var(--s1)", stroke: "var(--panel)", "stroke-width": 2, visibility: "hidden" });
    const d2 = sv("circle", { r: 4, fill: "var(--s2)", stroke: "var(--panel)", "stroke-width": 2, visibility: "hidden" });
    const hit = sv("rect", { x: m.l, y: m.t, width: iw, height: ih, fill: "transparent" });
    svg.append(cross, d1, d2, hit);
    const tip = el("div", { class: "tip", hidden: true });
    box.replaceChildren(svg, tip);
    const move = (ev) => {
      const r = svg.getBoundingClientRect();
      const px = (ev.clientX - r.left) * (W / r.width);
      const i = Math.max(0, Math.min(last, Math.round(((px - m.l) / iw) * last)));
      const p = data[i];
      for (const n of [cross, d1, d2]) n.setAttribute("visibility", "visible");
      cross.setAttribute("x1", x(i)); cross.setAttribute("x2", x(i));
      d1.setAttribute("cx", x(i)); d1.setAttribute("cy", y(p.open));
      d2.setAttribute("cx", x(i)); d2.setAttribute("cy", y(p.p0Open));
      tip.hidden = false;
      tip.replaceChildren(el("div", {}, fmtDayLong(p.day)),
        el("div", { class: "row2" }, el("i", { style: "--c: var(--s1)" }), "Открыто ", el("b", {}, String(p.open))),
        el("div", { class: "row2" }, el("i", { style: "--c: var(--s2)" }), "P0 ", el("b", {}, String(p.p0Open))),
        el("div", { class: "row2" }, "Сделано всего ", el("b", {}, String(p.done))));
      placeTip(box, tip, x(i) * (r.width / W), y(p.open) * (r.height / H));
    };
    const leave = () => { tip.hidden = true; for (const n of [cross, d1, d2]) n.setAttribute("visibility", "hidden"); };
    hit.addEventListener("pointermove", move);
    hit.addEventListener("pointerleave", leave);
  }

  function drawWeekly() {
    const box = $("cWeek");
    const data = stats.weekly;
    if (tableView.has("week")) {
      chartTable(box, ["Неделя с", "Закрыто"], [...data].reverse().map(w => [fmtDayLong(w.weekStart), w.done]));
      return;
    }
    const W = Math.max(box.clientWidth, 240), H = 180;
    const m = { l: 28, r: 6, t: 14, b: 26 };
    const iw = W - m.l - m.r, ih = H - m.t - m.b;
    const ticks = niceTicks(Math.max(...data.map(w => w.done), 1), 3);
    const ymax = ticks[ticks.length - 1];
    const y = (v) => m.t + ih - (v / ymax) * ih;
    const slot = iw / data.length, bw = Math.min(28, slot - 6);
    const svg = sv("svg", { viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": `Закрыто по неделям, последние ${data.length}: ${data.map(w => w.done).join(", ")}` });
    for (const v of ticks) {
      svg.append(sv("line", { class: v === 0 ? "axis" : "gridl", x1: m.l, x2: m.l + iw, y1: y(v), y2: y(v) }));
      svg.append(sv("text", { x: m.l - 6, y: y(v) + 4, "text-anchor": "end" }, String(v)));
    }
    const tip = el("div", { class: "tip", hidden: true });
    data.forEach((w, i) => {
      const cx = m.l + slot * i + slot / 2;
      const h = (w.done / ymax) * ih;
      if (w.done > 0) {
        const r = Math.min(4, bw / 2, h);
        const x0 = cx - bw / 2, y0 = m.t + ih - h, yb = m.t + ih;
        svg.append(sv("path", { fill: "var(--s1)", d: `M${x0},${yb}V${y0 + r}Q${x0},${y0} ${x0 + r},${y0}H${x0 + bw - r}Q${x0 + bw},${y0} ${x0 + bw},${y0 + r}V${yb}Z` }));
      }
      if (i % 2 === (data.length - 1) % 2) svg.append(sv("text", { x: cx, y: H - 6, "text-anchor": "middle" }, fmtDay(w.weekStart)));
      const hit = sv("rect", { x: m.l + slot * i, y: m.t, width: slot, height: ih, fill: "transparent" });
      hit.addEventListener("pointerenter", () => {
        tip.hidden = false;
        tip.replaceChildren(el("div", {}, "Неделя с " + fmtDayLong(w.weekStart)), el("div", { class: "row2" }, el("i", { style: "--c: var(--s1)" }), "Закрыто ", el("b", {}, String(w.done))));
        const r = svg.getBoundingClientRect();
        placeTip(box, tip, cx * (r.width / W), (m.t + ih - h) * (r.height / H));
      });
      hit.addEventListener("pointerleave", () => { tip.hidden = true; });
      svg.append(hit);
    });
    box.replaceChildren(svg, tip);
  }

  function drawHBars(box, rows, emptyText) {
    const max = Math.max(...rows.map(r => r.value), 0);
    if (!rows.length || max === 0) { box.replaceChildren(el("div", { class: "empty" }, emptyText)); return; }
    box.replaceChildren(...rows.map(r => el("div", { class: "hbar", title: `${r.name}: ${r.value} открыто` + (r.note ? `, сделано ${r.note}` : "") },
      el("span", { class: "name" }, r.name),
      el("div", { class: "track" }, el("div", { class: "fill", style: `width:${r.value ? Math.max(r.value / max * 100, 2) : 0}%` })),
      el("span", { class: "val" }, el("b", {}, String(r.value)), r.note ? ` · ${r.note}` : ""))));
  }

  function drawNextUp() {
    const box = $("nextUp");
    if (!stats.nextUp.length) { box.replaceChildren(el("div", { class: "empty" }, "Открытых задач нет.")); return; }
    box.replaceChildren(...stats.nextUp.map(i => el("div", { class: "li" },
      el("span", { class: "code" }, `${i.priority} ${i.code}`),
      el("button", { class: "t", type: "button", title: i.title, onclick: () => openItem(i.id) }, i.title),
      el("span", { class: "pill", style: `--st:${(STATUS[i.status] || STATUS.todo).color}` }, (STATUS[i.status] || STATUS.todo).label))));
  }

  function drawFeed() {
    const box = $("feed");
    if (!stats.recent.length) { box.replaceChildren(el("div", { class: "empty" }, "Изменений пока не было. Смените статус задачи — оно появится здесь.")); return; }
    box.replaceChildren(...stats.recent.map(c => el("div", { class: "li" },
      el("span", { class: "code" }, c.code),
      el("button", { class: "t", type: "button", title: c.title, onclick: () => openItem(c.itemId) },
        c.title, el("span", { class: "chg" }, ` — ${FIELD_LABEL[c.field] || c.field}: ${valLabel(c.field, c.old)} → ${valLabel(c.field, c.new)}`)),
      el("span", { class: "when", title: fmtDate(c.at) }, ago(c.at)))));
  }

  // ---------------------------------------------------------------- tasks list
  function matches(i) {
    if (ui.prio !== "all" && i.priority !== ui.prio) return false;
    if (ui.status !== "all" && i.status !== ui.status) return false;
    if (ui.kind !== "all" && i.kind !== ui.kind) return false;
    if (ui.hideDone && (i.status === "done" || i.status === "wontfix")) return false;
    const q = ui.q.trim().toLowerCase();
    if (q) { const hay = [i.code, i.title, i.area, i.evidence, i.fix, i.notes].join(" ").toLowerCase(); if (!hay.includes(q)) return false; }
    return true;
  }
  const statusOrder = { doing: 0, todo: 1, blocked: 2, done: 3, wontfix: 4 };
  function render() {
    renderSummary();
    $("cntItems").textContent = itemsLoaded ? String(items.length) : "";
    const root = $("groups");
    if (!itemsLoaded) return;
    root.replaceChildren();
    if (!items.length) { root.append(el("div", { class: "group" }, el("div", { class: "empty" }, "Задач пока нет. Добавьте первую через форму ниже."))); return; }
    let shown = 0;
    for (const p of Object.keys(PRIO)) {
      const all = items.filter(i => i.priority === p);
      const list = all.filter(matches).sort((a, b) =>
        (statusOrder[a.status] ?? 9) - (statusOrder[b.status] ?? 9) || (SEV[a.severity]?.rank ?? 9) - (SEV[b.severity]?.rank ?? 9) || (a.order ?? 0) - (b.order ?? 0));
      if (!list.length) continue;
      shown += list.length;
      const done = all.filter(i => i.status === "done").length;
      const g = el("section", { class: "group", "aria-label": p },
        el("div", { class: "ghead" }, el("h2", {}, el("span", { class: "p" }, p), el("span", {}, PRIO[p])), el("span", { class: "cnt" }, `${done}/${all.filter(i => i.status !== "wontfix").length} сделано`)));
      for (const i of list) g.append(rowEl(i));
      root.append(g);
    }
    if (!shown) root.append(el("div", { class: "group" }, el("div", { class: "empty" }, "Под фильтр ничего не попало. Сбросьте поиск или выберите «Все».")));
  }

  function rowEl(i) {
    const sev = SEV[i.severity] || SEV.low;
    const isOpen = open.has(i.id);
    const sel = el("select", { class: "status", "aria-label": `Статус ${i.code}`, style: `--st:${(STATUS[i.status] || STATUS.todo).color}`,
      onclick: (e) => e.stopPropagation(),
      onchange: (e) => patchItem(i, { status: e.target.value }) });
    for (const [k, v] of Object.entries(STATUS)) sel.append(el("option", { value: k, selected: k === i.status }, v.label));
    const row = el("article", { class: "row" + (isOpen ? " open" : "") + (i.status === "done" ? " isdone" : ""), style: `--sev:${sev.color}`, "data-id": i.id });
    const head = el("div", { class: "rhead", role: "button", tabindex: "0", "aria-expanded": String(isOpen),
      onclick: () => { if (isOpen) open.delete(i.id); else { open.add(i.id); loadHistory(i.id); } render(); },
      onkeydown: (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); head.click(); } } },
      el("span", { class: "stripe" }),
      el("span", { class: "code mono" }, i.code || ""),
      el("div", { class: "ttl" }, el("b", {}, i.title || "Без названия"),
        el("div", { class: "tags" }, el("span", { class: "tag sev" }, sev.label), i.kind ? el("span", { class: "tag" }, KIND[i.kind] || i.kind) : null, i.area ? el("span", { class: "tag mono" }, i.area) : null)),
      sel,
      el("span", { class: "caret", "aria-hidden": "true" }, "▸"));
    row.append(head);
    if (isOpen) row.append(detailEl(i));
    return row;
  }

  async function loadHistory(id) {
    try { history.set(id, await api("GET", "/api/items/" + encodeURIComponent(id) + "/changes")); render(); } catch {}
  }

  function detailEl(i) {
    const notes = el("textarea", { id: "notes-" + i.id, rows: "3", placeholder: "Заметки: что сделано, коммит, ссылки…" });
    notes.value = i.notes || "";
    notes.addEventListener("change", () => { if (notes.value !== (i.notes || "")) patchItem(i, { notes: notes.value }); });
    const prio = el("select", { id: "prio-" + i.id, onchange: (e) => patchItem(i, { priority: e.target.value }) });
    for (const p of Object.keys(PRIO)) prio.append(el("option", { value: p, selected: p === i.priority }, p));
    let confirmDel = false;
    const del = el("button", { class: "btn ghost", type: "button", onclick: async () => {
      if (!confirmDel) { confirmDel = true; del.textContent = "Точно удалить?"; del.className = "btn danger"; return; }
      try { await api("DELETE", "/api/items/" + encodeURIComponent(i.id)); open.delete(i.id); items = items.filter(x => x.id !== i.id); render(); loadStats(); }
      catch (e) { notice("Не удалось удалить: " + e.message, true); }
    } }, "Удалить");
    const hist = history.get(i.id) || [];
    return el("div", { class: "detail" },
      i.evidence ? el("div", { class: "field" }, el("span", { class: "k" }, "Доказательства"), el("div", { class: "v" }, i.evidence)) : null,
      i.fix ? el("div", { class: "field" }, el("span", { class: "k" }, "Что сделать"), el("div", { class: "v" }, i.fix)) : null,
      i.impact ? el("div", { class: "field full" }, el("span", { class: "k" }, "Влияние на клиента"), el("div", { class: "v" }, i.impact)) : null,
      el("div", { class: "field full" }, el("label", { class: "k", for: "notes-" + i.id }, "Заметки"), notes),
      hist.length ? el("div", { class: "field full" }, el("span", { class: "k" }, "История"),
        el("div", { class: "hist" }, hist.map(c => el("div", {}, `${fmtDate(c.at)} · ${FIELD_LABEL[c.field] || c.field}: ${valLabel(c.field, c.old)} → ${valLabel(c.field, c.new)}`)))) : null,
      el("div", { class: "dactions full" },
        el("span", { style: "display:flex;gap:8px;align-items:center" }, el("label", { for: "prio-" + i.id }, "Приоритет"), prio),
        el("span", {}, i.updatedAt ? "Изменено " + fmtDate(i.updatedAt) : ""),
        del));
  }

  // ---------------------------------------------------------------- product map
  function renderFeatures() {
    $("cntFeatures").textContent = featuresLoaded ? String(features.length) : "";
    if (!featuresLoaded) return;
    const body = $("featBody"); body.replaceChildren();
    if (!features.length) { body.append(el("tr", {}, el("td", { colspan: "4", class: "empty" }, "Карта продукта пуста."))); return; }
    for (const f of features) {
      const sel = el("select", { class: "status", "aria-label": `Статус: ${f.name}`, style: `--st:${FEAT[f.status] || "var(--fg)"}`,
        onchange: async (e) => {
          const v = e.target.value;
          try { await api("PATCH", "/api/features/" + encodeURIComponent(f.id), { status: v }); f.status = v; notice(""); }
          catch (err) { notice("Не удалось сохранить: " + err.message, true); }
          renderFeatures();
        } });
      for (const [k, v] of Object.entries(FEAT_LABEL)) sel.append(el("option", { value: k, selected: k === f.status }, v));
      body.append(el("tr", {}, el("td", {}, el("b", {}, f.name || "")), el("td", {}, sel), el("td", { class: "ev" }, f.evidence || ""), el("td", { class: "ev" }, f.limits || "")));
    }
  }

  // ---------------------------------------------------------------- add
  $("addForm").addEventListener("submit", async (e) => {
    e.preventDefault();
    const title = $("aTitle").value.trim();
    if (!title) { $("aMsg").textContent = "Введите название."; return; }
    $("aSubmit").disabled = true; $("aMsg").textContent = "Сохраняю…";
    try {
      const created = await api("POST", "/api/items", { code: $("aCode").value.trim(), title, priority: $("aPrio").value, severity: $("aSev").value, kind: $("aKind").value, area: $("aArea").value, fix: $("aFix").value });
      items.push(created);
      $("addForm").reset(); resetAdd();
      $("aMsg").textContent = `Добавлено: ${created.code}.`;
      render(); loadStats();
    } catch (err) { $("aMsg").textContent = err.message; }
    $("aSubmit").disabled = false;
  });

  // ---------------------------------------------------------------- load
  async function loadStats() {
    try { stats = await api("GET", "/api/stats?days=" + ui.days); renderDashboard(); }
    catch (e) { notice("Не удалось посчитать дашборд: " + e.message, true); }
  }
  async function load() {
    try {
      const [it, ft] = await Promise.all([api("GET", "/api/items"), api("GET", "/api/features")]);
      items = it; features = ft; itemsLoaded = featuresLoaded = true;
      sync("live", "Локальная база SQLite");
      notice("");
      render(); renderFeatures();
      await loadStats();
    } catch (e) {
      sync("off", "Сервер недоступен");
      notice("Не удалось загрузить данные. Запустите панель: make devpanel", true);
    }
  }

  let rt;
  window.addEventListener("resize", () => { clearTimeout(rt); rt = setTimeout(renderDashboard, 120); });
  // Another tab may have changed the board: refresh when this one gets focus.
  window.addEventListener("focus", () => { if (!document.querySelector("textarea:focus, input:focus")) load(); });

  showTab(currentTab());
  render(); renderFeatures();
  load();
})();
