// ── Helpers ───────────────────────────────────────────────────────────────────
const $ = id => {
  const el = document.getElementById(id);
  if (!el) console.error('[launcher] Element not found: #' + id);
  return el;
};
function log(fn, msg, ...args)    { console.log('[' + fn + ']', msg, ...args); }
function logErr(fn, msg, ...args) { console.error('[' + fn + '] ERROR:', msg, ...args); }

// ── Layout toggle ─────────────────────────────────────────────────────────────
const LAYOUT_KEY = 'launcher_layout';

function isVertical() {
  const v = document.getElementById('layout-vertical');
  return v ? !v.classList.contains('hidden') : true;
}

function syncLayouts(fromVert) {
  const fields = [
    ['port',              'h-port'],
    ['host',              'h-host'],
    ['db_path',           'h-db_path'],
    ['anthropic_api_key', 'h-anthropic_api_key'],
    ['openai_api_key',    'h-openai_api_key'],
    ['gemini_api_key',    'h-gemini_api_key'],
    ['ollama_base_url',   'h-ollama_base_url'],
    ['ollama_model',      'h-ollama_model'],
    ['ollama_timeout',    'h-ollama_timeout'],
  ];
  if (fromVert) {
    fields.forEach(([vId, hId]) => {
      const src = document.getElementById(vId), dst = document.getElementById(hId);
      if (src && dst) dst.value = src.value;
    });
    const vMode = document.querySelector('input[name="analysis_mode"]:checked');
    if (vMode) {
      const hMode = document.querySelector('input[name="h-analysis_mode"][value="' + vMode.value + '"]');
      if (hMode) hMode.checked = true;
    }
    const vLogs = document.getElementById('show_more_logs');
    const hLogs = document.getElementById('h-show_more_logs');
    if (vLogs && hLogs) hLogs.checked = vLogs.checked;
  } else {
    fields.forEach(([vId, hId]) => {
      const src = document.getElementById(hId), dst = document.getElementById(vId);
      if (src && dst) dst.value = src.value;
    });
    const hMode = document.querySelector('input[name="h-analysis_mode"]:checked');
    if (hMode) {
      const vMode = document.querySelector('input[name="analysis_mode"][value="' + hMode.value + '"]');
      if (vMode) vMode.checked = true;
    }
    const hLogs = document.getElementById('h-show_more_logs');
    const vLogs = document.getElementById('show_more_logs');
    if (hLogs && vLogs) vLogs.checked = hLogs.checked;
  }
}

function applyLayout(layout) {
  const v    = document.getElementById('layout-vertical');
  const h    = document.getElementById('layout-horizontal');
  const card = document.getElementById('main-card');
  const btn  = document.getElementById('layout-toggle');
  if (layout === 'horizontal') {
    if (v) v.classList.add('hidden');
    if (h) h.classList.remove('hidden');
    if (card) card.classList.add('card--wide');
    if (btn) btn.classList.add('btn-layout--active');
  } else {
    if (h) h.classList.add('hidden');
    if (v) v.classList.remove('hidden');
    if (card) card.classList.remove('card--wide');
    if (btn) btn.classList.remove('btn-layout--active');
  }
}

function toggleLayout() {
  if (isVertical()) {
    syncLayouts(true);
    applyLayout('horizontal');
    localStorage.setItem(LAYOUT_KEY, 'horizontal');
    log('toggleLayout', 'switched to horizontal');
  } else {
    syncLayouts(false);
    applyLayout('vertical');
    localStorage.setItem(LAYOUT_KEY, 'vertical');
    log('toggleLayout', 'switched to vertical');
  }
}

// ── Active layout field helpers ───────────────────────────────────────────────
function getActiveValue(vertId, horizId) {
  const id = isVertical() ? vertId : horizId;
  return (document.getElementById(id) || {value: ''}).value;
}

function getActiveChecked(vertId, horizId) {
  const id = isVertical() ? vertId : horizId;
  const el = document.getElementById(id);
  return el ? el.checked : false;
}

function getActiveMode() {
  const name = isVertical() ? 'analysis_mode' : 'h-analysis_mode';
  const el = document.querySelector('input[name="' + name + '"]:checked') ||
             document.querySelector('input[name="' + name + '"][checked]');
  return el ? el.value : 'standard';
}

// ── Health checks ─────────────────────────────────────────────────────────────
function getFormValues() {
  return {
    db_path:      getActiveValue('db_path',           'h-db_path'),
    ollama_url:   getActiveValue('ollama_base_url',   'h-ollama_base_url'),
    api_key:      getActiveValue('anthropic_api_key', 'h-anthropic_api_key'),
    openai_key:   getActiveValue('openai_api_key',    'h-openai_api_key'),
    gemini_key:   getActiveValue('gemini_api_key',    'h-gemini_api_key'),
  };
}

function updateHealthRow(id, result) {
  const el = document.getElementById(id);
  if (!el) { logErr('updateHealthRow', 'row not found: #' + id); return; }
  el.className = 'health-row ' + result.status;
  el.querySelector('.health-msg').textContent = result.message;
  log('health', id + ' → ' + result.status + ': ' + result.message);
}

function populateModelSelect(selId, models) {
  const sel = document.getElementById(selId);
  if (!sel || !models || models.length === 0) return;
  const current = sel.value;
  sel.innerHTML = '';
  models.forEach(m => {
    const opt = document.createElement('option');
    opt.value = m; opt.textContent = m;
    if (m === current) opt.selected = true;
    sel.appendChild(opt);
  });
  if (!models.includes(current) && current) {
    const opt = document.createElement('option');
    opt.value = current; opt.textContent = current + ' (not found)';
    opt.selected = true;
    sel.insertBefore(opt, sel.firstChild);
  }
}

async function runHealthChecks() {
  log('runHealthChecks', 'running...');
  const v = getFormValues();
  const params = new URLSearchParams({
    db_path:    v.db_path,
    ollama_url: v.ollama_url,
    api_key:    v.api_key,
    openai_key: v.openai_key,
    gemini_key: v.gemini_key,
  });
  try {
    const res  = await fetch('/health?' + params);
    if (!res.ok) { logErr('runHealthChecks', 'HTTP ' + res.status); return; }
    const data = await res.json();
    log('runHealthChecks', 'response:', data);

    updateHealthRow('health-sqlite',      data.sqlite);
    updateHealthRow('health-ollama',      data.ollama);
    updateHealthRow('health-anthropic',   data.anthropic);
    updateHealthRow('health-openai',      data.openai);
    updateHealthRow('health-gemini',      data.gemini);
    updateHealthRow('h-health-sqlite',    data.sqlite);
    updateHealthRow('h-health-ollama',    data.ollama);
    updateHealthRow('h-health-anthropic', data.anthropic);
    updateHealthRow('h-health-openai',    data.openai);
    updateHealthRow('h-health-gemini',    data.gemini);

    if (data.models && data.models.length > 0) {
      log('runHealthChecks', 'populating ' + data.models.length + ' models');
      populateModelSelect('ollama_model',   data.models);
      populateModelSelect('h-ollama_model', data.models);
    } else {
      log('runHealthChecks', 'no Ollama models available');
    }
  } catch(e) {
    logErr('runHealthChecks', 'fetch threw:', e);
  }
}

let healthTimer;
function scheduleHealthCheck() {
  clearTimeout(healthTimer);
  healthTimer = setTimeout(runHealthChecks, 600);
}

// ── App state ─────────────────────────────────────────────────────────────────
let currentAppUrl = '';

// ── Start app ─────────────────────────────────────────────────────────────────
async function startApp() {
  log('startApp', 'clicked');

  const port     = getActiveValue('port',              'h-port');
  const host     = getActiveValue('host',              'h-host');
  const dbPath   = getActiveValue('db_path',           'h-db_path');
  const apiKey    = getActiveValue('anthropic_api_key', 'h-anthropic_api_key');
  const openaiKey = getActiveValue('openai_api_key',    'h-openai_api_key');
  const geminiKey = getActiveValue('gemini_api_key',    'h-gemini_api_key');
  const ollamaU   = getActiveValue('ollama_base_url',   'h-ollama_base_url');
  const model     = getActiveValue('ollama_model',      'h-ollama_model');
  const timeout   = getActiveValue('ollama_timeout',    'h-ollama_timeout');
  const mode      = getActiveMode();
  const showLogs  = getActiveChecked('show_more_logs',  'h-show_more_logs');

  const maskedKey = apiKey ? apiKey.substring(0, 12) + '...' : 'empty';
  log('startApp', 'sending: port=' + port + ' model=' + model + ' key=' + maskedKey);

  ['start-btn', 'h-start-btn'].forEach(id => {
    const btn = document.getElementById(id);
    if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> Starting…'; }
  });

  const fd = new FormData();
  fd.append('port',              port);
  fd.append('host',              host);
  fd.append('db_path',           dbPath);
  fd.append('anthropic_api_key', apiKey);
  fd.append('openai_api_key',    openaiKey);
  fd.append('gemini_api_key',    geminiKey);
  fd.append('ollama_base_url',   ollamaU);
  fd.append('ollama_model',      model);
  fd.append('ollama_timeout',    timeout);
  fd.append('analysis_mode',     mode);
  if (showLogs) fd.append('show_more_logs', 'true');

  try {
    log('startApp', 'POST /start');
    const res  = await fetch('/start', { method: 'POST', body: fd });
    log('startApp', 'response status=' + res.status);

    if (!res.ok) {
      const text = await res.text();
      logErr('startApp', 'server error ' + res.status + ': ' + text);
      ['start-btn', 'h-start-btn'].forEach(id => {
        const btn = document.getElementById(id);
        if (btn) { btn.disabled = false; btn.innerHTML = '▶ &nbsp;Start Job Matcher'; }
      });
      alert('Server error ' + res.status + ': ' + text);
      return;
    }

    const data = await res.json();
    log('startApp', 'response:', data);

    if (data.ok) {
      currentAppUrl = data.url;
      log('startApp', 'success! app URL: ' + currentAppUrl);
      setRunningState(currentAppUrl);
      setTimeout(() => {
        log('startApp', 'opening browser: ' + currentAppUrl);
        window.open(currentAppUrl, '_blank');
      }, 800);
    } else {
      logErr('startApp', 'server returned ok=false:', data);
      ['start-btn', 'h-start-btn'].forEach(id => {
        const btn = document.getElementById(id);
        if (btn) { btn.disabled = false; btn.innerHTML = '▶ &nbsp;Start Job Matcher'; }
      });
    }
  } catch(e) {
    logErr('startApp', 'fetch threw:', e);
    ['start-btn', 'h-start-btn'].forEach(id => {
      const btn = document.getElementById(id);
      if (btn) { btn.disabled = false; btn.innerHTML = '▶ &nbsp;Start Job Matcher'; }
    });
    alert('Failed to start: ' + e);
  }
}

// ── Stop app ──────────────────────────────────────────────────────────────────
async function stopApp() {
  log('stopApp', 'clicked');
  if (!confirm('Stop the Job Matcher server?')) return;

  ['stop-btn', 'h-stop-btn'].forEach(id => {
    const btn = document.getElementById(id);
    if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span>'; }
  });

  try {
    log('stopApp', 'POST /stop');
    const res = await fetch('/stop', { method: 'POST' });
    log('stopApp', 'response status=' + res.status);

    if (res.ok) {
      log('stopApp', 'server stopped');
      setStoppedState();
    } else {
      logErr('stopApp', 'server error ' + res.status);
      ['stop-btn', 'h-stop-btn'].forEach(id => {
        const btn = document.getElementById(id);
        if (btn) { btn.disabled = false; btn.innerHTML = '■ &nbsp;Stop'; }
      });
    }
  } catch(e) {
    logErr('stopApp', 'fetch threw:', e);
    ['stop-btn', 'h-stop-btn'].forEach(id => {
      const btn = document.getElementById(id);
      if (btn) { btn.disabled = false; btn.innerHTML = '■ &nbsp;Stop'; }
    });
  }
}

// ── Restart app ───────────────────────────────────────────────────────────────
async function restartApp() {
  log('restartApp', 'clicked');

  const model = getActiveValue('ollama_model', 'h-ollama_model');
  const port  = getActiveValue('port', 'h-port');
  if (!confirm('Restart Job Matcher?\n\nNew model: ' + model + '\nNew port: ' + port)) return;

  ['restart-btn', 'h-restart-btn'].forEach(id => {
    const btn = document.getElementById(id);
    if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> Restarting…'; }
  });

  const host     = getActiveValue('host',              'h-host');
  const dbPath   = getActiveValue('db_path',           'h-db_path');
  const apiKey    = getActiveValue('anthropic_api_key', 'h-anthropic_api_key');
  const openaiKey = getActiveValue('openai_api_key',    'h-openai_api_key');
  const geminiKey = getActiveValue('gemini_api_key',    'h-gemini_api_key');
  const ollamaU   = getActiveValue('ollama_base_url',   'h-ollama_base_url');
  const timeout   = getActiveValue('ollama_timeout',    'h-ollama_timeout');
  const mode      = getActiveMode();
  const showLogs  = getActiveChecked('show_more_logs',  'h-show_more_logs');

  const maskedKey = apiKey ? apiKey.substring(0, 12) + '...' : 'empty';
  log('restartApp', 'new config: port=' + port + ' model=' + model + ' key=' + maskedKey);

  const fd = new FormData();
  fd.append('port',              port);
  fd.append('host',              host);
  fd.append('db_path',           dbPath);
  fd.append('anthropic_api_key', apiKey);
  fd.append('openai_api_key',    openaiKey);
  fd.append('gemini_api_key',    geminiKey);
  fd.append('ollama_base_url',   ollamaU);
  fd.append('ollama_model',      model);
  fd.append('ollama_timeout',    timeout);
  fd.append('analysis_mode',     mode);
  if (showLogs) fd.append('show_more_logs', 'true');

  try {
    log('restartApp', 'POST /restart');
    const res  = await fetch('/restart', { method: 'POST', body: fd });
    log('restartApp', 'response status=' + res.status);

    if (!res.ok) {
      const text = await res.text();
      logErr('restartApp', 'server error ' + res.status + ': ' + text);
      ['restart-btn', 'h-restart-btn'].forEach(id => {
        const btn = document.getElementById(id);
        if (btn) { btn.disabled = false; btn.innerHTML = '↺ &nbsp;Restart'; }
      });
      return;
    }

    const data = await res.json();
    log('restartApp', 'response:', data);

    if (data.ok) {
      currentAppUrl = data.url;
      log('restartApp', 'restarted at: ' + currentAppUrl);
      if (data.analysis_mode) {
        ['analysis_mode', 'h-analysis_mode'].forEach(name => {
          const el = document.querySelector('input[name="' + name + '"][value="' + data.analysis_mode + '"]');
          if (el) el.checked = true;
        });
      }
      setRunningState(currentAppUrl);
      setTimeout(() => window.open(currentAppUrl, '_blank'), 1000);
    } else {
      logErr('restartApp', 'server returned ok=false:', data);
      ['restart-btn', 'h-restart-btn'].forEach(id => {
        const btn = document.getElementById(id);
        if (btn) { btn.disabled = false; btn.innerHTML = '↺ &nbsp;Restart'; }
      });
    }
  } catch(e) {
    logErr('restartApp', 'fetch threw:', e);
    ['restart-btn', 'h-restart-btn'].forEach(id => {
      const btn = document.getElementById(id);
      if (btn) { btn.disabled = false; btn.innerHTML = '↺ &nbsp;Restart'; }
    });
  }
}

// ── Open app ──────────────────────────────────────────────────────────────────
function openApp() {
  log('openApp', 'opening: ' + currentAppUrl);
  if (currentAppUrl) {
    window.open(currentAppUrl, '_blank');
  } else {
    logErr('openApp', 'no app URL set');
  }
}

// ── UI state helpers ──────────────────────────────────────────────────────────
function setRunningState(url) {
  log('setRunningState', 'url=' + url);
  ['', 'h-'].forEach(p => {
    const startBtn     = document.getElementById(p + 'start-btn');
    const runningPanel = document.getElementById(p + 'running-panel');
    const urlText      = document.getElementById(p + 'url-text');
    const urlLink      = document.getElementById(p + 'url-link');
    const restartBtn   = document.getElementById(p + 'restart-btn');
    const stopBtn      = document.getElementById(p + 'stop-btn');
    const restartNote  = document.getElementById(p + 'restart-note');

    if (startBtn) {
      startBtn.innerHTML = '✓ &nbsp;Running';
      startBtn.style.background   = 'var(--green)';
      startBtn.style.color        = '#fff';
      startBtn.style.borderColor  = 'var(--green)';
      startBtn.disabled = true;
    }
    if (urlText)      urlText.textContent = url;
    if (urlLink)      urlLink.href = url;
    if (runningPanel) runningPanel.classList.remove('hidden');
    if (restartBtn)   { restartBtn.disabled = false; restartBtn.innerHTML = '↺ &nbsp;Restart'; }
    if (stopBtn)      { stopBtn.disabled = false; stopBtn.innerHTML = '■ &nbsp;Stop'; }
    if (restartNote)  restartNote.classList.remove('hidden');
  });
}

function setStoppedState() {
  log('setStoppedState', 'resetting to start state');
  currentAppUrl = '';
  ['', 'h-'].forEach(p => {
    const startBtn     = document.getElementById(p + 'start-btn');
    const runningPanel = document.getElementById(p + 'running-panel');
    const restartNote  = document.getElementById(p + 'restart-note');

    if (startBtn) {
      startBtn.innerHTML          = '▶ &nbsp;Start Job Matcher';
      startBtn.style.background   = '';
      startBtn.style.color        = '';
      startBtn.style.borderColor  = '';
      startBtn.disabled = false;
    }
    if (runningPanel) runningPanel.classList.add('hidden');
    if (restartNote)  restartNote.classList.add('hidden');
  });
}

// ── Init ──────────────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', () => {
  log('init', 'DOMContentLoaded fired');
  const savedLayout = localStorage.getItem(LAYOUT_KEY) || 'horizontal';
  log('init', 'applying layout: ' + savedLayout);
  applyLayout(savedLayout);
  runHealthChecks();
});
