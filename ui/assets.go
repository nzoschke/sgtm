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
  grid-template-rows: minmax(250px, 32vh) auto minmax(0, 1fr);
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
.reading, .side, .soundCheck, .chartShell {
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
.soundCheck {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: end;
  gap: 12px;
  padding: 10px 12px;
  min-height: 0;
}
.zoneFields {
  display: grid;
  grid-template-columns: repeat(4, minmax(92px, 1fr));
  gap: 10px;
  min-width: 0;
}
.zoneInput {
  display: grid;
  gap: 4px;
  min-width: 0;
}
.zoneInput span {
  color: var(--muted);
  font-size: .74rem;
  text-transform: uppercase;
  letter-spacing: 0;
}
.zoneInput input {
  width: 100%;
  min-width: 0;
  height: 36px;
  border: 1px solid #26313a;
  border-radius: 6px;
  padding: 0 10px;
  background: #0b1014;
  color: var(--text);
  font: 700 1.05rem/1 system-ui;
}
.zoneActions {
  display: grid;
  grid-template-columns: auto auto minmax(86px, auto);
  gap: 8px;
  align-items: center;
}
.zoneActions button {
  height: 36px;
  border: 1px solid #26313a;
  border-radius: 6px;
  padding: 0 12px;
  background: #151d24;
  color: var(--text);
  font: 700 .95rem/1 system-ui;
}
.zoneActions button:first-child {
  background: #58a6ff;
  border-color: #58a6ff;
  color: #04101f;
}
.configStatus {
  color: var(--muted);
  font-size: .85rem;
  overflow-wrap: anywhere;
}
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
  .app { grid-template-rows: minmax(360px, 44vh) auto minmax(0, 1fr); }
  .top { grid-template-columns: 1fr; }
  .reading { padding: 20px 24px; }
  .side { padding: 12px 18px; }
  .stats { gap: 4px 14px; }
  .label { font-size: .68rem; }
  .value { font-size: 5.25rem; }
  .unit { font-size: 2rem; }
  .band { font-size: 1.65rem; }
  .metric { font-size: 1.25rem; }
  .metric.small { font-size: .9rem; }
  .unit { padding-bottom: 0; }
  .soundCheck { grid-template-columns: 1fr; align-items: stretch; }
  .zoneFields { grid-template-columns: 1fr 1fr; }
  .zoneActions { grid-template-columns: 1fr 1fr; }
  .configStatus { grid-column: 1 / -1; }
}
`

const DashboardJS = `
const defaultCfg = {idealMax: 85, unsafeMin: 95, chartMin: 35, chartMax: 120, historySec: 1800};
let cfg = {...defaultCfg};
const signalGapMs = 5000;
let readings = [];
let sessionStarted = null;
const el = id => document.getElementById(id);
const canvas = el('chart');
const ctx = canvas.getContext('2d');

function setState(state) {
  if (state.config) {
    cfg = {...cfg, ...state.config};
    applyConfigToInputs();
  }
  if (state.history) {
    readings = state.history;
  } else if (state.reading) {
    addReading(state.reading);
  }
  if (state.session) sessionStarted = Date.parse(state.session);
  if (state.status) setStatus(state.status);
  update();
}

function applyConfigToInputs() {
  el('idealMaxInput').value = formatInput(cfg.idealMax);
  el('unsafeMinInput').value = formatInput(cfg.unsafeMin);
  el('chartMinInput').value = formatInput(cfg.chartMin);
  el('chartMaxInput').value = formatInput(cfg.chartMax);
  updateZoneLegend();
}

function formatInput(value) {
  return Number.isInteger(value) ? String(value) : String(Number(value.toFixed(1)));
}

function configFromInputs() {
  const next = {
    ...cfg,
    idealMax: Number(el('idealMaxInput').value),
    unsafeMin: Number(el('unsafeMinInput').value),
    chartMin: Number(el('chartMinInput').value),
    chartMax: Number(el('chartMaxInput').value),
  };
  validateConfig(next);
  return next;
}

function validateConfig(next) {
  for (const key of ['idealMax', 'unsafeMin', 'chartMin', 'chartMax']) {
    if (!Number.isFinite(next[key]) || next[key] < 0 || next[key] > 180) {
      throw new Error('Use 0-180 dB');
    }
  }
  if (!(next.chartMin < next.idealMax && next.idealMax < next.unsafeMin && next.unsafeMin < next.chartMax)) {
    throw new Error('Need min < green < red < max');
  }
}

function previewConfig() {
  try {
    cfg = configFromInputs();
    setConfigStatus('Unsaved');
    update();
  } catch (error) {
    setConfigStatus(error.message);
  }
}

async function saveConfig(next = null) {
  try {
    cfg = next || configFromInputs();
    const response = await fetch('/api/config', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(cfg),
    });
    if (!response.ok) throw new Error(await response.text());
    cfg = {...cfg, ...(await response.json())};
    applyConfigToInputs();
    setConfigStatus('Saved');
    update();
  } catch (error) {
    setConfigStatus(String(error.message || error).trim());
  }
}

function resetConfig() {
  const next = {...cfg, ...defaultCfg};
  cfg = next;
  applyConfigToInputs();
  saveConfig(next);
}

function setConfigStatus(status) {
  el('configStatus').textContent = status;
}

function updateZoneLegend() {
  el('configStatus').textContent = 'Green <= ' + formatInput(cfg.idealMax) + ' | Red >= ' + formatInput(cfg.unsafeMin);
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
  const yStep = ch < 220 ? 20 : 10;
  for (let v = Math.ceil(min / yStep) * yStep; v <= max; v += yStep) {
    const yy = y(v);
    ctx.beginPath();
    ctx.moveTo(x0, yy);
    ctx.lineTo(x0 + cw, yy);
    ctx.stroke();
    ctx.fillText(String(v), 14, yy + 5);
  }
  const now = Date.now();
  const start = now - cfg.historySec * 1000;
  const x = t => x0 + ((typeof t === 'number' ? t : Date.parse(t)) - start) / (cfg.historySec * 1000) * cw;
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
  let previous = null;
  let moved = false;
  for (const r of readings) {
    const xx = x(r.time);
    const yy = y(r.display);
    if (xx < x0 || xx > x0 + cw || !Number.isFinite(yy)) continue;
    const currentTime = Date.parse(r.time);
    const previousTime = previous ? Date.parse(previous.time) : NaN;
    if (!moved || !Number.isFinite(previousTime) || currentTime - previousTime > signalGapMs) {
      ctx.moveTo(xx, yy);
      moved = true;
    } else {
      ctx.lineTo(xx, yy);
    }
    previous = r;
  }
  if (moved) ctx.stroke();
  drawSignalGaps(x, x0, y0 + ch, cw);
  ctx.strokeStyle = '#5b6670';
  ctx.strokeRect(x0, y0, cw, ch);
  ctx.fillStyle = '#f6f8fa';
  ctx.font = '16px system-ui';
  ctx.fillText('Ideal', x0 + 12, y(cfg.idealMax) + 26);
  ctx.fillText('Too High', x0 + 12, y0 + 26);
}

function drawSignalGaps(x, x0, y, cw) {
  ctx.save();
  ctx.strokeStyle = 'rgba(247,250,252,.55)';
  ctx.lineWidth = 3;
  ctx.beginPath();
  let drew = false;
  for (let i = 1; i < readings.length; i++) {
    const previous = readings[i - 1];
    const current = readings[i];
    const previousTime = Date.parse(previous.time);
    const currentTime = Date.parse(current.time);
    if (!Number.isFinite(previousTime) || !Number.isFinite(currentTime) || currentTime - previousTime <= signalGapMs) {
      continue;
    }
    const from = Math.max(x0, x(previousTime + signalGapMs));
    const to = Math.min(x0 + cw, x(currentTime - signalGapMs));
    if (to <= from) continue;
    ctx.moveTo(from, y);
    ctx.lineTo(to, y);
    drew = true;
  }
  if (drew) ctx.stroke();
  ctx.restore();
}

function formatTime(ms) {
  const date = new Date(ms);
  return date.toLocaleTimeString([], {hour: 'numeric', minute: '2-digit'});
}

window.addEventListener('resize', update);
for (const id of ['idealMaxInput', 'unsafeMinInput', 'chartMinInput', 'chartMaxInput']) {
  el(id).addEventListener('input', previewConfig);
}
el('saveConfig').addEventListener('click', () => saveConfig());
el('resetConfig').addEventListener('click', resetConfig);
applyConfigToInputs();
fetch('/api/state').then(r => r.json()).then(setState);
const events = new EventSource('/events');
events.onmessage = ev => setState(JSON.parse(ev.data));
setInterval(update, 1000);
`
