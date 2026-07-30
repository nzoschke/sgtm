package ui

const DashboardCSS = `
:root {
  color-scheme: dark;
  --bg: #07090b;
  --panel: #10161b;
  --text: #f6f8fa;
  --muted: #99a4ad;
  --green: #2ec46f;
  --amber: #e2a32d;
  --red: #ef4444;
  --line: #f7fafc;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  height: 100vh;
  background: var(--bg);
  color: var(--text);
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  overflow: hidden;
}
.app {
  height: 100vh;
  display: grid;
  grid-template-rows: minmax(250px, 34vh) minmax(0, 1fr);
  gap: 14px;
  padding: 18px;
  overflow: hidden;
}
.top {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(220px, .6fr);
  gap: 14px;
  align-items: stretch;
  min-height: 0;
}
.reading, .side, .chartShell {
  background: var(--panel);
  border: 1px solid #26313a;
  border-radius: 8px;
}
.reading {
  padding: 24px;
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 14px;
  align-items: center;
  min-height: 0;
  overflow: hidden;
}
.value {
  font-size: 8.75rem;
  font-weight: 800;
  line-height: .82;
  letter-spacing: 0;
}
.unit {
  font-size: 3rem;
  color: var(--muted);
  font-weight: 700;
  padding-bottom: .08em;
}
.band {
  grid-column: 1 / -1;
  font-size: 2.2rem;
  font-weight: 800;
  letter-spacing: 0;
  align-self: end;
}
.band.ideal { color: var(--green); }
.band.watch { color: var(--amber); }
.band.unsafe { color: var(--red); }
.side {
  padding: 14px 18px;
  min-height: 0;
  overflow: hidden;
}
.stats {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px 16px;
  align-content: start;
  min-height: 0;
}
.stats > div {
  min-width: 0;
}
.label {
  color: var(--muted);
  font-size: .78rem;
  text-transform: uppercase;
  letter-spacing: 0;
}
.metric {
  font-size: 1.6rem;
  font-weight: 750;
  line-height: 1.05;
}
.metric.small {
  font-size: 1.05rem;
  overflow-wrap: anywhere;
}
.statusTile {
  grid-column: 1 / -1;
}
.status {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.dot {
  width: 12px;
  height: 12px;
  flex: 0 0 auto;
  border-radius: 999px;
  background: var(--amber);
}
.dot.live { background: var(--green); }
.dot.retrying { background: var(--red); }
.chartShell {
  min-height: 0;
  padding: 12px;
  display: grid;
  overflow: hidden;
}
canvas {
  width: 100%;
  height: 100%;
  min-height: 0;
  display: block;
}
@media (max-width: 800px) {
  .app { padding: 16px; }
  .app { grid-template-rows: minmax(420px, 55vh) minmax(0, 1fr); }
  .top { grid-template-columns: 1fr; }
  .reading { padding: 20px 24px; }
  .value { font-size: 5.25rem; }
  .unit { font-size: 2rem; }
  .band { font-size: 1.65rem; }
  .metric { font-size: 1.45rem; }
  .metric.small { font-size: 1rem; }
  .unit { padding-bottom: 0; }
}
`

const DashboardJS = `
let cfg = {idealMax: 85, unsafeMin: 95, chartMin: 35, chartMax: 120, historySec: 1800};
let readings = [];
let sessionStarted = null;
const el = id => document.getElementById(id);
const canvas = el('chart');
const ctx = canvas.getContext('2d');

function setState(state) {
  if (state.config) cfg = state.config;
  if (state.history) {
    readings = state.history;
  } else if (state.reading) {
    addReading(state.reading);
  }
  if (state.session) sessionStarted = Date.parse(state.session);
  if (state.status) setStatus(state.status);
  update();
}

function addReading(reading) {
  readings.push(reading);
  const cutoff = Date.now() - cfg.historySec * 1000;
  readings = readings.filter(r => Date.parse(r.time) >= cutoff);
}

function setStatus(status) {
  el('status').textContent = status;
  el('dot').className = 'dot ' + status;
}

function bandFor(v) {
  if (!Number.isFinite(v)) return ['Waiting', ''];
  if (v >= cfg.unsafeMin) return ['Too High', 'unsafe'];
  if (v > cfg.idealMax) return ['Watch', 'watch'];
  return ['Ideal', 'ideal'];
}

function update() {
  const latest = readings[readings.length - 1];
  if (latest) {
    el('value').textContent = latest.display.toFixed(1);
    el('unit').textContent = latest.unit;
    const [label, cls] = bandFor(latest.display);
    el('band').textContent = label;
    el('band').className = 'band ' + cls;
    el('battery').textContent = latest.lowPower ? 'Low' : 'OK';
    el('autoOff').textContent = latest.autoPowerOff ? 'Enabled' : 'Disabled';
    el('range').textContent = latest.rangeLow + '-' + latest.rangeHigh;
    el('meterState').textContent = latest.hold ? 'Hold' : (latest.overload !== 'none' ? latest.overload : 'Live');
  }
  const values = readings.map(r => r.display).filter(Number.isFinite);
  const peak = values.length ? Math.max(...values) : NaN;
  const avg = values.length ? values.reduce((a, b) => a + b, 0) / values.length : NaN;
  el('peak').textContent = Number.isFinite(peak) ? peak.toFixed(1) : '--.-';
  el('avg').textContent = Number.isFinite(avg) ? avg.toFixed(1) : '--.-';
  el('window').textContent = Math.round(cfg.historySec / 60) + ' min';
  el('session').textContent = sessionStarted ? formatDuration(Date.now() - sessionStarted) : '--';
  draw();
}

function formatDuration(ms) {
  const total = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return h + 'h ' + String(m).padStart(2, '0') + 'm';
  return m + ':' + String(s).padStart(2, '0');
}

function resizeCanvas() {
  const rect = canvas.getBoundingClientRect();
  const dpr = window.devicePixelRatio || 1;
  canvas.width = Math.max(1, Math.floor(rect.width * dpr));
  canvas.height = Math.max(1, Math.floor(rect.height * dpr));
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
}

function draw() {
  resizeCanvas();
  const w = canvas.clientWidth;
  const h = canvas.clientHeight;
  const pad = {l: 58, r: 18, t: 18, b: 54};
  const x0 = pad.l, y0 = pad.t, cw = w - pad.l - pad.r, ch = h - pad.t - pad.b;
  ctx.clearRect(0, 0, w, h);
  ctx.fillStyle = '#0b1014';
  ctx.fillRect(0, 0, w, h);
  const min = cfg.chartMin, max = cfg.chartMax;
  const y = v => y0 + (max - v) / (max - min) * ch;
  ctx.fillStyle = 'rgba(239,68,68,.22)';
  ctx.fillRect(x0, y0, cw, Math.max(0, y(cfg.unsafeMin) - y0));
  ctx.fillStyle = 'rgba(226,163,45,.18)';
  ctx.fillRect(x0, y(cfg.unsafeMin), cw, Math.max(0, y(cfg.idealMax) - y(cfg.unsafeMin)));
  ctx.fillStyle = 'rgba(46,196,111,.18)';
  ctx.fillRect(x0, y(cfg.idealMax), cw, Math.max(0, y0 + ch - y(cfg.idealMax)));
  ctx.strokeStyle = '#26313a';
  ctx.lineWidth = 1;
  ctx.fillStyle = '#99a4ad';
  ctx.font = '14px system-ui';
  for (let v = Math.ceil(min / 10) * 10; v <= max; v += 10) {
    const yy = y(v);
    ctx.beginPath();
    ctx.moveTo(x0, yy);
    ctx.lineTo(x0 + cw, yy);
    ctx.stroke();
    ctx.fillText(String(v), 14, yy + 5);
  }
  const now = Date.now();
  const start = now - cfg.historySec * 1000;
  const x = t => x0 + (Date.parse(t) - start) / (cfg.historySec * 1000) * cw;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';
  ctx.fillStyle = '#99a4ad';
  ctx.font = '13px system-ui';
  for (let i = 0; i <= 4; i++) {
    const tickTime = start + cfg.historySec * 1000 * (i / 4);
    const xx = x0 + cw * (i / 4);
    const label = formatTime(tickTime);
    ctx.beginPath();
    ctx.moveTo(xx, y0 + ch);
    ctx.lineTo(xx, y0 + ch + 6);
    ctx.stroke();
    ctx.textAlign = i === 0 ? 'left' : (i === 4 ? 'right' : 'center');
    ctx.fillText(label, xx, y0 + ch + 10);
  }
  ctx.textAlign = 'left';
  ctx.textBaseline = 'alphabetic';
  ctx.strokeStyle = 'rgba(247,250,252,.95)';
  ctx.lineWidth = 3;
  ctx.beginPath();
  let moved = false;
  for (const r of readings) {
    const xx = x(r.time);
    const yy = y(r.display);
    if (xx < x0 || xx > x0 + cw || !Number.isFinite(yy)) continue;
    if (!moved) {
      ctx.moveTo(xx, yy);
      moved = true;
    } else {
      ctx.lineTo(xx, yy);
    }
  }
  if (moved) ctx.stroke();
  ctx.strokeStyle = '#5b6670';
  ctx.strokeRect(x0, y0, cw, ch);
  ctx.fillStyle = '#f6f8fa';
  ctx.font = '16px system-ui';
  ctx.fillText('Ideal', x0 + 12, y(cfg.idealMax) + 26);
  ctx.fillText('Too High', x0 + 12, y0 + 26);
}

function formatTime(ms) {
  const date = new Date(ms);
  return date.toLocaleTimeString([], {hour: 'numeric', minute: '2-digit'});
}

window.addEventListener('resize', update);
fetch('/api/state').then(r => r.json()).then(setState);
const events = new EventSource('/events');
events.onmessage = ev => setState(JSON.parse(ev.data));
setInterval(update, 1000);
`
