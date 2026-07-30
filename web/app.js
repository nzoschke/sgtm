const SERVICE_UUID = '0000fff0-0000-1000-8000-00805f9b34fb';
const WRITE_UUID = '0000fff1-0000-1000-8000-00805f9b34fb';
const NOTIFY_UUID = '0000fff2-0000-1000-8000-00805f9b34fb';
const START_COMMAND = Uint8Array.from([0xd5, 0xfc, 0x11, 0x0d]);
const DB_NAME = 'sgtm-web';
const STORE_NAME = 'readings';
const HISTORY_MS = 30 * 60 * 1000;
const MAX_POINTS = 5000;
const CHART_MIN = 35;
const CHART_MAX = 120;
const IDEAL_MAX = 85;
const UNSAFE_MIN = 95;
const SIGNAL_GAP_MS = 5000;

const els = {
  connect: document.querySelector('#connect'),
  disconnect: document.querySelector('#disconnect'),
  clear: document.querySelector('#clear'),
  status: document.querySelector('#status'),
  dot: document.querySelector('#dot'),
  band: document.querySelector('#band'),
  value: document.querySelector('#value'),
  unit: document.querySelector('#unit'),
  avg: document.querySelector('#avg'),
  peak: document.querySelector('#peak'),
  samples: document.querySelector('#samples'),
  device: document.querySelector('#device'),
  range: document.querySelector('#range'),
  overload: document.querySelector('#overload'),
  battery: document.querySelector('#battery'),
  autoOff: document.querySelector('#autoOff'),
  backlight: document.querySelector('#backlight'),
  chart: document.querySelector('#chart'),
};

let bluetoothDevice;
let writeCharacteristic;
let notifyCharacteristic;
let readings = [];
let frameBuffer = [];
let database;

els.connect.addEventListener('click', connect);
els.disconnect.addEventListener('click', disconnect);
els.clear.addEventListener('click', clearHistory);
window.addEventListener('resize', drawChart);

init();

async function init() {
  if (!('bluetooth' in navigator)) {
    setStatus('Web Bluetooth is not available in this browser. Use Chrome on macOS.');
    els.connect.disabled = true;
    return;
  }
  database = await openDatabase();
  readings = await loadRecentReadings();
  pruneReadings();
  updateDisplay(readings.at(-1));
  drawChart();
}

async function connect() {
  try {
    setStatus('Choose the 850019 EM device in Chrome');
    bluetoothDevice = await navigator.bluetooth.requestDevice({
      filters: [{ namePrefix: '850019' }, { services: [SERVICE_UUID] }],
      optionalServices: [SERVICE_UUID],
    });
    bluetoothDevice.addEventListener('gattserverdisconnected', onDisconnected);
    els.device.textContent = bluetoothDevice.name || bluetoothDevice.id || 'Selected meter';

    setStatus('Connecting');
    const server = await bluetoothDevice.gatt.connect();
    const service = await server.getPrimaryService(SERVICE_UUID);
    writeCharacteristic = await service.getCharacteristic(WRITE_UUID);
    notifyCharacteristic = await service.getCharacteristic(NOTIFY_UUID);
    notifyCharacteristic.addEventListener('characteristicvaluechanged', onNotification);
    await notifyCharacteristic.startNotifications();
    await writeStartCommand();

    els.connect.disabled = true;
    els.disconnect.disabled = false;
    setStatus('Live');
  } catch (error) {
    setStatus(error.message || String(error));
    disconnect();
  }
}

function disconnect() {
  if (notifyCharacteristic) {
    notifyCharacteristic.removeEventListener('characteristicvaluechanged', onNotification);
  }
  if (bluetoothDevice?.gatt?.connected) {
    bluetoothDevice.gatt.disconnect();
  }
  notifyCharacteristic = undefined;
  writeCharacteristic = undefined;
  els.connect.disabled = false;
  els.disconnect.disabled = true;
}

function onDisconnected() {
  els.connect.disabled = false;
  els.disconnect.disabled = true;
  setStatus('Disconnected');
}

async function writeStartCommand() {
  if ('writeValueWithoutResponse' in writeCharacteristic) {
    await writeCharacteristic.writeValueWithoutResponse(START_COMMAND);
    return;
  }
  await writeCharacteristic.writeValue(START_COMMAND);
}

function onNotification(event) {
  const chunk = new Uint8Array(event.target.value.buffer);
  for (const reading of pushFrame(chunk)) {
    addReading(reading);
  }
}

async function addReading(reading) {
  readings.push(reading);
  pruneReadings();
  updateDisplay(reading);
  drawChart();
  if (database) {
    await saveReading(reading);
  }
}

function pruneReadings() {
  const cutoff = Date.now() - HISTORY_MS;
  readings = readings.filter((reading) => reading.time >= cutoff).slice(-MAX_POINTS);
}

function updateDisplay(reading) {
  els.samples.textContent = String(readings.length);
  if (!reading) {
    els.value.textContent = '--.-';
    els.unit.textContent = 'dBA';
    els.avg.textContent = '--.-';
    els.peak.textContent = '--.-';
    els.range.textContent = '--';
    els.overload.textContent = '--';
    els.battery.textContent = '--';
    els.autoOff.textContent = '--';
    els.backlight.textContent = '--';
    setBand();
    return;
  }
  els.value.textContent = reading.display.toFixed(1);
  els.unit.textContent = reading.unit;
  els.range.textContent = `${reading.rangeLow}-${reading.rangeHigh}`;
  els.overload.textContent = reading.overload;
  els.battery.textContent = reading.lowPower ? 'Low' : 'OK';
  els.autoOff.textContent = reading.autoPowerOff ? 'Enabled' : 'Disabled';
  els.backlight.textContent = reading.backlight ? 'On' : 'Off';
  setBand(reading.display);

  const values = readings.map((item) => item.display);
  const sum = values.reduce((total, value) => total + value, 0);
  els.avg.textContent = (sum / values.length).toFixed(1);
  els.peak.textContent = Math.max(...values).toFixed(1);
}

function drawChart() {
  const canvas = els.chart;
  const rect = canvas.getBoundingClientRect();
  const dpr = window.devicePixelRatio || 1;
  canvas.width = Math.max(1, Math.floor(rect.width * dpr));
  canvas.height = Math.max(1, Math.floor(rect.height * dpr));
  const ctx = canvas.getContext('2d');
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  const width = rect.width;
  const height = rect.height;
  const pad = { left: 52, right: 16, top: 16, bottom: 30 };
  const plotW = width - pad.left - pad.right;
  const plotH = height - pad.top - pad.bottom;
  const now = Date.now();
  const start = now - HISTORY_MS;

  ctx.clearRect(0, 0, width, height);
  fillBand(ctx, pad, plotW, plotH, CHART_MIN, IDEAL_MAX, 'rgba(41, 184, 111, 0.18)');
  fillBand(ctx, pad, plotW, plotH, IDEAL_MAX, UNSAFE_MIN, 'rgba(215, 161, 43, 0.16)');
  fillBand(ctx, pad, plotW, plotH, UNSAFE_MIN, CHART_MAX, 'rgba(230, 75, 75, 0.18)');

  ctx.strokeStyle = '#2a353e';
  ctx.lineWidth = 1;
  ctx.beginPath();
  for (let value = 40; value <= 120; value += 10) {
    const y = yFor(value, pad, plotH);
    ctx.moveTo(pad.left, y);
    ctx.lineTo(width - pad.right, y);
    ctx.fillStyle = '#9ca7af';
    ctx.font = '12px system-ui';
    ctx.fillText(String(value), 12, y + 4);
  }
  ctx.stroke();

  if (readings.length > 1) {
    ctx.strokeStyle = '#f5f7f8';
    ctx.lineWidth = 3;
    ctx.beginPath();
    let previous;
    let moved = false;
    readings.forEach((reading) => {
      const x = pad.left + ((reading.time - start) / HISTORY_MS) * plotW;
      const y = yFor(reading.display, pad, plotH);
      if (!moved || !previous || reading.time - previous.time > SIGNAL_GAP_MS) {
        ctx.moveTo(x, y);
        moved = true;
      } else {
        ctx.lineTo(x, y);
      }
      previous = reading;
    });
    ctx.stroke();
    drawSignalGaps(ctx, pad, plotW, plotH, start);
  }

  ctx.fillStyle = '#9ca7af';
  ctx.font = '12px system-ui';
  ctx.fillText('30 min', pad.left, height - 8);
  ctx.fillText('now', width - pad.right - 26, height - 8);
}

function drawSignalGaps(ctx, pad, plotW, plotH, start) {
  ctx.save();
  ctx.strokeStyle = 'rgba(245, 247, 248, 0.55)';
  ctx.lineWidth = 3;
  ctx.beginPath();
  let drew = false;
  for (let index = 1; index < readings.length; index += 1) {
    const previous = readings[index - 1];
    const current = readings[index];
    if (current.time - previous.time <= SIGNAL_GAP_MS) {
      continue;
    }
    const from = pad.left + ((previous.time + SIGNAL_GAP_MS - start) / HISTORY_MS) * plotW;
    const to = pad.left + ((current.time - SIGNAL_GAP_MS - start) / HISTORY_MS) * plotW;
    if (to <= from) {
      continue;
    }
    const y = pad.top + plotH;
    ctx.moveTo(Math.max(pad.left, from), y);
    ctx.lineTo(Math.min(pad.left + plotW, to), y);
    drew = true;
  }
  if (drew) {
    ctx.stroke();
  }
  ctx.restore();
}

function fillBand(ctx, pad, plotW, plotH, low, high, color) {
  const y1 = yFor(high, pad, plotH);
  const y2 = yFor(low, pad, plotH);
  ctx.fillStyle = color;
  ctx.fillRect(pad.left, y1, plotW, y2 - y1);
}

function yFor(value, pad, plotH) {
  const clamped = Math.max(CHART_MIN, Math.min(CHART_MAX, value));
  return pad.top + ((CHART_MAX - clamped) / (CHART_MAX - CHART_MIN)) * plotH;
}

function pushFrame(chunk) {
  frameBuffer.push(...chunk);
  const parsed = [];
  for (;;) {
    const start = frameBuffer.indexOf(0xd5);
    if (start < 0) {
      frameBuffer = [];
      return parsed;
    }
    if (start > 0) {
      frameBuffer = frameBuffer.slice(start);
    }
    if (frameBuffer.length < 2) {
      return parsed;
    }
    let packetLen = 0;
    if (frameBuffer[1] === 0xf0) {
      if (frameBuffer.length < 4) return parsed;
      packetLen = u16be(frameBuffer, 2) + 5;
    } else if (frameBuffer[1] === 0xa1) {
      if (frameBuffer.length < 3) return parsed;
      packetLen = frameBuffer[2] + 4;
    } else {
      frameBuffer = frameBuffer.slice(1);
      continue;
    }
    if (frameBuffer.length < packetLen) {
      return parsed;
    }
    const packet = frameBuffer.slice(0, packetLen);
    frameBuffer = frameBuffer.slice(packetLen);
    if (packet.at(-1) !== 0x0d) {
      continue;
    }
    const reading = parseDT95(packet);
    if (reading) {
      parsed.push(reading);
    }
  }
}

function parseDT95(packet) {
  if (packet.length < 11 || packet[0] !== 0xd5 || packet[1] !== 0xf0) {
    return null;
  }
  const n = u16be(packet, 2);
  const data = packet.slice(4, 4 + n);
  if (data.length < 6) {
    return null;
  }
  const flags = data[4];
  const status = data[5];
  const range = dt95Range(flags);
  return {
    time: Date.now(),
    value: u16be(data, 0) / 10,
    display: u16be(data, 2) / 10,
    unit: flags & 0x04 ? 'dBC' : 'dBA',
    rangeLow: range[0],
    rangeHigh: range[1],
    overload: flags & 0x40 ? 'high' : flags & 0x20 ? 'low' : 'none',
    maxMin: flags & 0x03 ? (flags & 0x03) === 1 ? 'max' : 'min' : 'none',
    lowPower: Boolean(status & 0x08),
    autoPowerOff: Boolean(status & 0x04),
    backlight: Boolean(status & 0x02),
    hold: Boolean(status & 0x01),
  };
}

function dt95Range(flags) {
  switch ((flags >> 3) & 0x03) {
    case 1:
      return [80, 130];
    case 2:
      return [35, 130];
    case 3:
      return [35, 80];
    default:
      return [50, 100];
  }
}

function u16be(bytes, offset) {
  return (bytes[offset] << 8) | bytes[offset + 1];
}

function setStatus(message) {
  els.status.textContent = message;
  const status = message.toLowerCase();
  els.dot.className = status === 'live' ? 'dot live' : status.includes('disconnect') || status.includes('error') ? 'dot error' : 'dot';
}

function setBand(value) {
  if (value === undefined) {
    els.band.textContent = 'Waiting';
    els.band.className = 'band';
    return;
  }
  if (value >= UNSAFE_MIN) {
    els.band.textContent = 'Too high';
    els.band.className = 'band unsafe';
  } else if (value > IDEAL_MAX) {
    els.band.textContent = 'Watch';
    els.band.className = 'band watch';
  } else {
    els.band.textContent = 'Ideal';
    els.band.className = 'band ideal';
  }
}

function openDatabase() {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, 1);
    request.onupgradeneeded = () => {
      const db = request.result;
      if (!db.objectStoreNames.contains(STORE_NAME)) {
        db.createObjectStore(STORE_NAME, { keyPath: 'time' });
      }
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

function loadRecentReadings() {
  return new Promise((resolve) => {
    const cutoff = Date.now() - HISTORY_MS;
    const transaction = database.transaction(STORE_NAME, 'readonly');
    const store = transaction.objectStore(STORE_NAME);
    const request = store.getAll(IDBKeyRange.lowerBound(cutoff));
    request.onsuccess = () => resolve(request.result || []);
    request.onerror = () => resolve([]);
  });
}

function saveReading(reading) {
  return new Promise((resolve) => {
    const transaction = database.transaction(STORE_NAME, 'readwrite');
    transaction.objectStore(STORE_NAME).put(reading);
    transaction.oncomplete = resolve;
    transaction.onerror = resolve;
  });
}

function clearHistory() {
  readings = [];
  updateDisplay();
  drawChart();
  if (!database) {
    return;
  }
  const transaction = database.transaction(STORE_NAME, 'readwrite');
  transaction.objectStore(STORE_NAME).clear();
}
