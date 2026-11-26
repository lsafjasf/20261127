package routing

const pageCSS = `
:root {
  --ink: #17202b;
  --muted: #657386;
  --line: #dce3ea;
  --paper: #f7f9fb;
  --card: #ffffff;
  --track: #2563eb;
  --bus: #0f766e;
  --send: #c56a0b;
  --danger: #dc2626;
  --cycle: #be123c;
}
* { box-sizing: border-box; }
html { color-scheme: light; }
body {
  margin: 0;
  color: var(--ink);
  background:
    radial-gradient(circle at 8% 0%, rgba(37, 99, 235, .09), transparent 26rem),
    radial-gradient(circle at 92% 10%, rgba(15, 118, 110, .08), transparent 24rem),
    var(--paper);
  font: 14px/1.5 ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif;
}
.shell { width: min(1480px, calc(100% - 40px)); margin: 28px auto 56px; }
.masthead { display: flex; justify-content: space-between; gap: 28px; align-items: flex-end; margin-bottom: 22px; }
.eyebrow { margin: 0 0 5px; color: var(--track); font: 700 11px/1.4 ui-monospace, SFMono-Regular, Consolas, monospace; letter-spacing: .14em; text-transform: uppercase; }
h1 { margin: 0; font-size: clamp(26px, 3.2vw, 42px); line-height: 1.08; letter-spacing: -.035em; }
.lead { max-width: 670px; margin: 9px 0 0; color: var(--muted); font-size: 15px; }
.summary-grid { display: grid; grid-template-columns: repeat(4, minmax(110px, 1fr)); gap: 10px; min-width: min(480px, 46vw); }
.metric { padding: 13px 15px; border: 1px solid var(--line); border-radius: 12px; background: rgba(255,255,255,.8); }
.metric strong { display: block; font: 700 24px/1.15 ui-monospace, SFMono-Regular, Consolas, monospace; }
.metric span { color: var(--muted); font-size: 11px; text-transform: uppercase; letter-spacing: .08em; }
.metric.bad strong { color: var(--danger); }
.toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 9px; margin: 0 0 12px; }
.search { flex: 1 1 310px; min-width: 220px; }
.search input {
  width: 100%; padding: 10px 13px; border: 1px solid var(--line); border-radius: 10px;
  background: var(--card); color: var(--ink); outline: none;
}
.search input:focus { border-color: #7aa2ef; box-shadow: 0 0 0 3px rgba(37,99,235,.12); }
.chip {
  padding: 9px 13px; border: 1px solid var(--line); border-radius: 999px; background: var(--card);
  color: var(--muted); cursor: pointer; font-weight: 600;
}
.chip:hover { border-color: #aab8c6; color: var(--ink); }
.chip.active { background: var(--ink); border-color: var(--ink); color: #fff; }
.legend { display: flex; flex-wrap: wrap; gap: 15px; margin-left: auto; color: var(--muted); font-size: 12px; }
.legend span { display: inline-flex; align-items: center; gap: 6px; }
.swatch { width: 22px; height: 0; border-top: 3px solid var(--track); }
.swatch.send { border-top-color: var(--send); border-top-style: dashed; }
.swatch.bad { border-top-color: var(--danger); border-top-style: dotted; }
.card { border: 1px solid var(--line); border-radius: 16px; background: var(--card); box-shadow: 0 14px 44px rgba(23,32,43,.055); }
.graph-card { overflow: hidden; margin-bottom: 16px; }
.graph-scroll { overflow: auto; padding: 10px; background-image: radial-gradient(#e8edf2 .7px, transparent .7px); background-size: 18px 18px; }
.routing-map { display: block; width: 100%; min-width: 1080px; height: auto; }
.node-box { fill: #fff; stroke: #cbd6e2; stroke-width: 1.2; filter: url(#node-shadow); }
.node-track .node-box { stroke: #93b4f8; }
.node-bus .node-box { stroke: #72b9b1; }
.node-badge { fill: var(--muted); font: 700 9px/1 ui-monospace, SFMono-Regular, Consolas, monospace; letter-spacing: .11em; }
.node-name { fill: var(--ink); font-size: 15px; font-weight: 720; }
.node-id { fill: #738195; font: 10px/1 ui-monospace, SFMono-Regular, Consolas, monospace; }
.node-divider { stroke: #e6ebf0; stroke-width: 1; }
.port-input { fill: #e3f5f2; stroke: var(--bus); stroke-width: 1.5; }
.port-output { fill: #e9effd; stroke: var(--track); stroke-width: 1.5; }
.node-bus .port-output { fill: #e3f5f2; stroke: var(--bus); }
.port-label { fill: #445266; font: 10.5px/1 ui-monospace, SFMono-Regular, Consolas, monospace; }
.port-direction { fill: #a0abb7; font: 700 8px/1 ui-monospace, SFMono-Regular, Consolas, monospace; letter-spacing: .08em; }
.route-edge { cursor: pointer; opacity: .82; transition: opacity .15s ease; }
.route-edge .edge-line { stroke: var(--track); stroke-width: 2.2; }
.route-edge .edge-source { fill: var(--track); }
.route-edge[data-kind="send"] .edge-line { stroke: var(--send); stroke-dasharray: 7 5; }
.route-edge[data-kind="send"] .edge-source { fill: var(--send); }
.route-edge[data-kind="unknown"] .edge-line { stroke: var(--danger); stroke-dasharray: 3 4; }
.route-edge.edge-invalid .edge-line { stroke: var(--danger) !important; stroke-dasharray: 3 4 !important; }
.route-edge.edge-invalid .edge-source { fill: var(--danger); }
.route-edge.edge-cycle .edge-line { stroke: var(--cycle) !important; stroke-dasharray: 2 5 !important; animation: cyclePulse 1.2s ease-in-out infinite; }
.route-edge:hover, .route-edge.is-selected { opacity: 1; }
.route-edge:hover .edge-line, .route-edge.is-selected .edge-line { stroke-width: 3.6; }
.edge-label { fill: #657386; stroke: #fff; stroke-width: 3.5px; paint-order: stroke fill; font: 9.5px/1 ui-monospace, SFMono-Regular, Consolas, monospace; pointer-events: none; }
.ghost-box { fill: #fff5f5; stroke: var(--danger); stroke-width: 1.5; stroke-dasharray: 5 4; }
.ghost-title { fill: var(--danger); font: 800 9px/1 ui-monospace, SFMono-Regular, Consolas, monospace; letter-spacing: .1em; }
.ghost-id { fill: #9f1e1e; font: 11px/1 ui-monospace, SFMono-Regular, Consolas, monospace; }
.is-dim { opacity: .14 !important; }
@keyframes cyclePulse { 50% { opacity: .4; } }
.section-title { display: flex; align-items: center; gap: 10px; margin: 22px 0 10px; }
.section-title h2 { margin: 0; font-size: 17px; letter-spacing: -.01em; }
.section-title span { color: var(--muted); font: 11px/1 ui-monospace, SFMono-Regular, Consolas, monospace; }
.issue-list { display: grid; gap: 9px; padding: 14px; }
.issue { display: grid; grid-template-columns: auto 1fr auto; gap: 11px; align-items: start; padding: 11px 13px; border: 1px solid #f0c7c7; border-radius: 10px; background: #fff8f8; }
.issue-code { padding: 2px 7px; border-radius: 5px; background: #fee2e2; color: #991b1b; font: 700 10px/1.5 ui-monospace, SFMono-Regular, Consolas, monospace; }
.issue-message { color: #5b2525; }
.issue-op { color: #9c6a6a; font: 10px/1.5 ui-monospace, SFMono-Regular, Consolas, monospace; }
.ok-panel { padding: 18px 20px; color: #166534; background: #f0fdf4; border: 1px solid #bbf7d0; border-radius: 12px; }
.table-card { overflow: auto; }
table { width: 100%; border-collapse: collapse; min-width: 900px; }
th, td { padding: 10px 13px; border-bottom: 1px solid #edf0f4; text-align: left; vertical-align: top; }
th { position: sticky; top: 0; z-index: 1; background: #f8fafc; color: var(--muted); font-size: 10px; text-transform: uppercase; letter-spacing: .08em; }
tbody tr:last-child td { border-bottom: 0; }
tbody tr:hover { background: #fafcff; }
tbody tr.is-selected { background: #eff5ff; }
td code { font: 11px/1.45 ui-monospace, SFMono-Regular, Consolas, monospace; color: #334155; overflow-wrap: anywhere; }
.kind { display: inline-flex; align-items: center; gap: 6px; padding: 3px 8px; border-radius: 999px; background: #eff5ff; color: #1d4ed8; font-size: 11px; font-weight: 700; white-space: nowrap; }
.kind.send { background: #fff5e8; color: #9a4d05; }
.status { font: 700 10px/1.6 ui-monospace, SFMono-Regular, Consolas, monospace; color: #15803d; }
.status.bad { color: var(--danger); }
.empty { padding: 24px; color: var(--muted); text-align: center; }
.footer { margin-top: 22px; color: var(--muted); font-size: 11px; }
@media (max-width: 900px) {
  .shell { width: min(100% - 24px, 1480px); margin-top: 18px; }
  .masthead { align-items: flex-start; flex-direction: column; }
  .summary-grid { width: 100%; min-width: 0; grid-template-columns: repeat(2, 1fr); }
  .legend { margin-left: 0; }
}
`

const svgCSS = `
.node-box { fill: #fff; stroke: #cbd6e2; stroke-width: 1.2; filter: url(#node-shadow); }
.node-track .node-box { stroke: #93b4f8; }
.node-bus .node-box { stroke: #72b9b1; }
.node-badge { fill: #657386; font-family: monospace; font-size: 9px; font-weight: 700; letter-spacing: .11em; }
.node-name { fill: #17202b; font-family: sans-serif; font-size: 15px; font-weight: 700; }
.node-id { fill: #738195; font-family: monospace; font-size: 10px; }
.node-divider { stroke: #e6ebf0; }
.port-input { fill: #e3f5f2; stroke: #0f766e; stroke-width: 1.5; }
.port-output { fill: #e9effd; stroke: #2563eb; stroke-width: 1.5; }
.node-bus .port-output { fill: #e3f5f2; stroke: #0f766e; }
.port-label { fill: #445266; font-family: monospace; font-size: 10.5px; }
.port-direction { fill: #a0abb7; font-family: monospace; font-size: 8px; font-weight: 700; }
.route-edge { opacity: .82; }
.route-edge .edge-line { fill: none; stroke: #2563eb; stroke-width: 2.2; }
.route-edge .edge-source { fill: #2563eb; }
.route-edge[data-kind="send"] .edge-line { stroke: #c56a0b; stroke-dasharray: 7 5; }
.route-edge[data-kind="send"] .edge-source { fill: #c56a0b; }
.route-edge.edge-invalid .edge-line { stroke: #dc2626; stroke-dasharray: 3 4; }
.route-edge.edge-invalid .edge-source { fill: #dc2626; }
.route-edge.edge-cycle .edge-line { stroke: #be123c; stroke-dasharray: 2 5; }
.edge-label { fill: #657386; stroke: #fff; stroke-width: 3.5px; paint-order: stroke fill; font-family: monospace; font-size: 9.5px; }
.ghost-box { fill: #fff5f5; stroke: #dc2626; stroke-width: 1.5; stroke-dasharray: 5 4; }
.ghost-title { fill: #dc2626; font-family: monospace; font-size: 9px; font-weight: 800; letter-spacing: .1em; }
.ghost-id { fill: #9f1e1e; font-family: monospace; font-size: 11px; }
`

const pageJS = `
(() => {
  const search = document.querySelector("#route-search");
  const chips = [...document.querySelectorAll("[data-filter]")];
  const rows = [...document.querySelectorAll("[data-row-index]")];
  const edges = [...document.querySelectorAll("[data-route-index]")];
  const nodes = [...document.querySelectorAll(".node")];
  let filter = "all";

  function routeMatches(element, query) {
    if (filter !== "all" && element.dataset.kind !== filter) return false;
    return !query || (element.dataset.search || "").includes(query);
  }

  function apply() {
    const query = (search?.value || "").trim().toLowerCase();
    edges.forEach((edge) => edge.classList.toggle("is-dim", !routeMatches(edge, query)));
    rows.forEach((row) => row.hidden = !routeMatches(row, query));
    nodes.forEach((node) => {
      const nodeMatch = !query || (node.dataset.search || "").includes(query);
      const connected = edges.some((edge) =>
        !edge.classList.contains("is-dim") &&
        (edge.dataset.fromNode === node.dataset.nodeId || edge.dataset.toNode === node.dataset.nodeId));
      node.classList.toggle("is-dim", Boolean(query) && !nodeMatch && !connected);
    });
  }

  chips.forEach((chip) => chip.addEventListener("click", () => {
    filter = chip.dataset.filter;
    chips.forEach((item) => item.classList.toggle("active", item === chip));
    apply();
  }));
  search?.addEventListener("input", apply);

  function select(index) {
    edges.forEach((edge) => edge.classList.toggle("is-selected", edge.dataset.routeIndex === index));
    rows.forEach((row) => row.classList.toggle("is-selected", row.dataset.rowIndex === index));
  }
  [...edges, ...rows].forEach((element) => {
    const index = element.dataset.routeIndex || element.dataset.rowIndex;
    element.addEventListener("click", () => select(index));
  });
})();
`
