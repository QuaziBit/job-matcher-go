package launcher

import (
	"fmt"
	"github.com/QuaziBit/job-matcher-go/config"
)

// checkedIf returns "checked" if condition is true, empty string otherwise.
func checkedIf(condition bool) string {
	if condition {
		return "checked"
	}
	return ""
}

func renderLauncherPage(cfg config.Config) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
<title>Job Matcher — Launcher</title>
<style>
@import url('https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500&family=IBM+Plex+Sans:wght@300;400;500;600&display=swap');
:root {
  --bg:#0d0e11; --bg2:#13151a; --bg3:#1c1f27;
  --border:#2a2d38; --border2:#383c4a;
  --text:#d4d8e8; --text-dim:#6b7080; --text-mute:#3d4050;
  --amber:#f59e0b; --green:#22c55e; --red:#ef4444; --yellow:#eab308;
  --radius:4px; --mono:'IBM Plex Mono',monospace; --sans:'IBM Plex Sans',sans-serif;
}
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0;}
body{background:var(--bg);color:var(--text);font-family:var(--sans);
  min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;}
.card{background:var(--bg2);border:1px solid var(--border);border-radius:6px;
  width:100%%;max-width:540px;overflow:hidden;}
.card-header{padding:20px 24px 16px;border-bottom:1px solid var(--border);}
.card-header h1{font-size:18px;font-weight:600;letter-spacing:-0.02em;}
.logo-mark{font-family:var(--mono);font-size:10px;color:var(--amber);
  letter-spacing:0.15em;text-transform:uppercase;margin-bottom:4px;}
.card-body{padding:24px;}
.section{margin-bottom:24px;}
.section-title{font-size:11px;font-weight:600;text-transform:uppercase;
  letter-spacing:0.1em;color:var(--text-dim);margin-bottom:12px;
  padding-bottom:6px;border-bottom:1px solid var(--border);}
.form-row{margin-bottom:14px;}
.toggle-group{display:flex;gap:8px;flex-wrap:wrap;}
.toggle-option{display:flex;flex-direction:column;flex:1;min-width:100px;border:1px solid var(--border);border-radius:var(--radius);padding:8px 10px;cursor:pointer;transition:border-color .15s;}
.toggle-option:has(input:checked){border-color:var(--amber);background:rgba(245,158,11,.06);}
.toggle-option input{display:none;}
.toggle-option span{font-size:12px;font-weight:600;color:var(--text);text-transform:none;letter-spacing:0;margin:0;}
.toggle-option small{font-size:10px;color:var(--text-dim);margin-top:3px;text-transform:none;letter-spacing:0;}
.form-grid{display:grid;grid-template-columns:1fr 1fr;gap:12px;}
label{display:block;font-size:11px;font-weight:500;color:var(--text-dim);
  letter-spacing:0.06em;text-transform:uppercase;margin-bottom:5px;}
input,select{background:var(--bg3);border:1px solid var(--border);color:var(--text);
  border-radius:var(--radius);padding:8px 12px;font-family:var(--sans);
  font-size:13px;width:100%%;outline:none;transition:border-color .15s;}
input:focus,select:focus{border-color:var(--amber);}
input::placeholder{color:var(--text-mute);}
select option{background:var(--bg3);}
.health-row{display:flex;align-items:flex-start;gap:10px;
  padding:10px 12px;background:var(--bg3);border:1px solid var(--border);
  border-radius:var(--radius);margin-bottom:8px;}
.health-icon{font-size:14px;flex-shrink:0;margin-top:1px;}
.health-label{font-size:11px;font-weight:600;text-transform:uppercase;
  letter-spacing:0.08em;color:var(--text-dim);font-family:var(--mono);}
.health-msg{font-size:12px;color:var(--text-dim);margin-top:2px;font-family:var(--mono);}
.ok .health-icon::before{content:'✓';color:var(--green);}
.warn .health-icon::before{content:'⚠';color:var(--yellow);}
.error .health-icon::before{content:'✗';color:var(--red);}
.loading .health-icon::before{content:'○';color:var(--text-mute);}
.btn{display:inline-flex;align-items:center;justify-content:center;gap:8px;
  padding:10px 20px;border-radius:var(--radius);border:1px solid transparent;
  font-family:var(--sans);font-size:14px;font-weight:500;cursor:pointer;
  transition:all .15s;width:100%%;}
.btn-primary{background:var(--amber);color:#0d0e11;border-color:var(--amber);}
.btn-primary:hover:not(:disabled){background:#fbbf24;}
.btn-primary:disabled{opacity:.45;cursor:not-allowed;}
.btn-warning{background:transparent;color:#f59e0b;border-color:#f59e0b;}
.btn-warning:hover:not(:disabled){background:rgba(245,158,11,.12);}
.btn-danger{background:transparent;color:var(--red);border-color:var(--red);}
.btn-danger:hover:not(:disabled){background:rgba(239,68,68,.12);}
.btn-ghost{background:transparent;color:var(--text-dim);border-color:var(--border);}
.btn-ghost:hover:not(:disabled){border-color:var(--amber);color:var(--amber);}
.spinner{width:14px;height:14px;border:2px solid rgba(0,0,0,.2);
  border-top-color:#0d0e11;border-radius:50%%;animation:spin .6s linear infinite;}
@keyframes spin{to{transform:rotate(360deg)}}
.url-display{display:flex;align-items:center;gap:10px;padding:10px 12px;
  background:var(--bg3);border:1px solid var(--border);border-radius:var(--radius);
  margin-top:14px;}
.url-text{font-family:var(--mono);font-size:12px;color:var(--amber);flex:1;}
.url-link{font-size:11px;color:var(--text-dim);text-decoration:none;
  padding:3px 8px;border:1px solid var(--border);border-radius:var(--radius);}
.url-link:hover{border-color:var(--amber);color:var(--amber);}
.hidden{display:none!important;}
</style>
</head>
<body>
<div class="card">
  <div class="card-header">
    <div class="logo-mark">// job-matcher</div>
    <h1>Launcher</h1>
  </div>
  <div class="card-body">

    <!-- Health Status -->
    <div class="section">
      <div class="section-title">System Status</div>
      <div id="health-sqlite"  class="health-row loading">
        <div class="health-icon"></div>
        <div><div class="health-label">SQLite</div><div class="health-msg">Checking…</div></div>
      </div>
      <div id="health-ollama"  class="health-row loading">
        <div class="health-icon"></div>
        <div><div class="health-label">Ollama</div><div class="health-msg">Checking…</div></div>
      </div>
      <div id="health-anthropic" class="health-row loading">
        <div class="health-icon"></div>
        <div><div class="health-label">Anthropic API</div><div class="health-msg">Checking…</div></div>
      </div>
    </div>

    <!-- Server -->
    <div class="section">
      <div class="section-title">Server</div>
      <div class="form-grid">
        <div class="form-row" style="margin:0;">
          <label>Port</label>
          <input type="number" id="port" value="%d" min="1024" max="65535"/>
        </div>
        <div class="form-row" style="margin:0;">
          <label>Host</label>
          <input type="text" id="host" value="%s"/>
        </div>
      </div>
    </div>

    <!-- Database -->
    <div class="section">
      <div class="section-title">Database</div>
      <div class="form-row">
        <label>SQLite Path</label>
        <input type="text" id="db_path" value="%s"
               placeholder="./job_matcher.db"
               oninput="scheduleHealthCheck()"/>
      </div>
    </div>

    <!-- LLM Providers -->
    <div class="section">
      <div class="section-title">LLM Providers</div>
      <div class="form-row">
        <label>Anthropic API Key</label>
        <input type="password" id="anthropic_api_key" value="%s"
               placeholder="sk-ant-..."
               oninput="scheduleHealthCheck()"/>
      </div>
      <div class="form-row">
        <label>Ollama URL</label>
        <input type="text" id="ollama_base_url" value="%s"
               oninput="scheduleHealthCheck()"/>
      </div>
      <div class="form-row">
        <label>Ollama Model</label>
        <select id="ollama_model">
          <option value="%s">%s</option>
        </select>
      </div>
      <div class="form-row">
        <label>Ollama Timeout (seconds)</label>
        <input type="number" id="ollama_timeout" value="%d" min="30"/>
      </div>
      <div class="form-row">
        <label>Analysis Mode</label>
        <div class="toggle-group">
          <label class="toggle-option">
            <input type="radio" name="analysis_mode" value="fast" %s/>
            <span>Fast</span>
            <small>~30s · short snippets · no suggestions</small>
          </label>
          <label class="toggle-option">
            <input type="radio" name="analysis_mode" value="standard" %s/>
            <span>Standard</span>
            <small>~90s · medium snippets · suggestions on</small>
          </label>
          <label class="toggle-option">
            <input type="radio" name="analysis_mode" value="detailed" %s/>
            <span>Detailed</span>
            <small>~4min · full snippets · all skills</small>
          </label>
        </div>
      </div>
    </div>

    <!-- Start -->
    <button class="btn btn-primary" id="start-btn" onclick="startApp()">
      ▶ &nbsp;Start Job Matcher
    </button>

    <!-- Running state panel (shown after Start) -->
    <div id="running-panel" class="hidden" style="margin-top:14px;">
      <div id="url-display" class="url-display" style="margin-bottom:10px;">
        <span class="url-text" id="url-text"></span>
        <a class="url-link" id="url-link" href="#" target="_blank">Open ↗</a>
      </div>
      <div style="display:flex;gap:8px;">
        <button class="btn btn-ghost" id="open-btn" onclick="openApp()" style="flex:1;">
          🌐 &nbsp;Open App
        </button>
        <button class="btn btn-warning" id="restart-btn" onclick="restartApp()" style="flex:1;">
          ↺ &nbsp;Restart
        </button>
        <button class="btn btn-danger" id="stop-btn" onclick="stopApp()" style="flex:1;">
          ■ &nbsp;Stop
        </button>
      </div>
      <div id="restart-note" class="hidden" style="margin-top:10px;font-size:11px;color:var(--text-dim);font-family:var(--mono);">
        Change model or port above, then click Restart to apply.
      </div>
    </div>

  </div>
</div>

<script>
console.log('[launcher] Script loaded');

// ── Helpers ───────────────────────────────────────────────────────────────────
const $ = id => {
  const el = document.getElementById(id);
  if (!el) console.error('[launcher] Element not found: #' + id);
  return el;
};
function log(fn, msg, ...args)  { console.log('[' + fn + ']', msg, ...args); }
function logErr(fn, msg, ...args) { console.error('[' + fn + '] ERROR:', msg, ...args); }

// ── Health checks ─────────────────────────────────────────────────────────────
function getFormValues() {
  return {
    db_path:   ($('db_path')           || {value:''}).value,
    ollama_url:($('ollama_base_url')   || {value:''}).value,
    api_key:   ($('anthropic_api_key') || {value:''}).value,
  };
}

function updateHealthRow(id, result) {
  const el = $(id);
  if (!el) { logErr('updateHealthRow', 'row not found: #' + id); return; }
  el.className = 'health-row ' + result.status;
  el.querySelector('.health-msg').textContent = result.message;
  log('health', id + ' → ' + result.status + ': ' + result.message);
}

async function runHealthChecks() {
  log('runHealthChecks', 'running...');
  const v = getFormValues();
  const params = new URLSearchParams({
    db_path:    v.db_path,
    ollama_url: v.ollama_url,
    api_key:    v.api_key,
  });
  try {
    const res  = await fetch('/health?' + params);
    if (!res.ok) {
      logErr('runHealthChecks', 'HTTP ' + res.status);
      return;
    }
    const data = await res.json();
    log('runHealthChecks', 'response:', data);

    updateHealthRow('health-sqlite',    data.sqlite);
    updateHealthRow('health-ollama',    data.ollama);
    updateHealthRow('health-anthropic', data.anthropic);

    // Populate model dropdown from live Ollama models
    const sel = $('ollama_model');
    if (!sel) { logErr('runHealthChecks', '#ollama_model not found'); return; }
    const current = sel.value;
    if (data.models && data.models.length > 0) {
      log('runHealthChecks', 'populating ' + data.models.length + ' models');
      sel.innerHTML = '';
      data.models.forEach(m => {
        const opt = document.createElement('option');
        opt.value = m; opt.textContent = m;
        if (m === current) opt.selected = true;
        sel.appendChild(opt);
      });
      if (!data.models.includes(current) && current) {
        log('runHealthChecks', 'current model not in list, adding: ' + current);
        const opt = document.createElement('option');
        opt.value = current; opt.textContent = current + ' (not found)';
        opt.selected = true;
        sel.insertBefore(opt, sel.firstChild);
      }
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
  const btn = $('start-btn');
  if (!btn) { logErr('startApp', '#start-btn not found'); return; }

  const port    = ($('port')             || {value:''}).value;
  const host    = ($('host')             || {value:''}).value;
  const dbPath  = ($('db_path')          || {value:''}).value;
  const apiKey  = ($('anthropic_api_key')|| {value:''}).value;
  const ollamaU = ($('ollama_base_url')  || {value:''}).value;
  const model   = ($('ollama_model')     || {value:''}).value;
  const timeout = ($('ollama_timeout')   || {value:''}).value;

  const maskedKey = apiKey ? apiKey.substring(0, 12) + '...' : 'empty';
  log('startApp', 'sending: port=' + port + ' model=' + model + ' key=' + maskedKey);

  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span> Starting…';

  const fd = new FormData();
  fd.append('port',              port);
  fd.append('host',              host);
  fd.append('db_path',           dbPath);
  fd.append('anthropic_api_key', apiKey);
  fd.append('ollama_base_url',   ollamaU);
  fd.append('ollama_model',      model);
  fd.append('ollama_timeout',    timeout);
  const modeElS = document.querySelector('input[name="analysis_mode"]:checked');
  fd.append('analysis_mode', modeElS ? modeElS.value : 'standard');

  try {
    log('startApp', 'POST /start');
    const res  = await fetch('/start', { method: 'POST', body: fd });
    log('startApp', 'response status=' + res.status);

    if (!res.ok) {
      const text = await res.text();
      logErr('startApp', 'server error ' + res.status + ': ' + text);
      btn.disabled = false;
      btn.innerHTML = '▶ &nbsp;Start Job Matcher';
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
      btn.disabled = false;
      btn.innerHTML = '▶ &nbsp;Start Job Matcher';
    }
  } catch(e) {
    logErr('startApp', 'fetch threw:', e);
    btn.disabled = false;
    btn.innerHTML = '▶ &nbsp;Start Job Matcher';
    alert('Failed to start: ' + e);
  }
}

// ── Stop app ──────────────────────────────────────────────────────────────────
async function stopApp() {
  log('stopApp', 'clicked');
  if (!confirm('Stop the Job Matcher server?')) return;

  const btn = $('stop-btn');
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span>'; }

  try {
    log('stopApp', 'POST /stop');
    const res = await fetch('/stop', { method: 'POST' });
    log('stopApp', 'response status=' + res.status);

    if (res.ok) {
      log('stopApp', 'server stopped');
      setStoppedState();
    } else {
      logErr('stopApp', 'server error ' + res.status);
      if (btn) { btn.disabled = false; btn.innerHTML = '■ &nbsp;Stop'; }
    }
  } catch(e) {
    logErr('stopApp', 'fetch threw:', e);
    if (btn) { btn.disabled = false; btn.innerHTML = '■ &nbsp;Stop'; }
  }
}

// ── Restart app ───────────────────────────────────────────────────────────────
async function restartApp() {
  log('restartApp', 'clicked');

  const model = ($('ollama_model') || {value:''}).value;
  const port  = ($('port')         || {value:''}).value;
  if (!confirm('Restart Job Matcher?\n\nNew model: ' + model + '\nNew port: ' + port)) return;

  const btn = $('restart-btn');
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> Restarting…'; }

  const port2    = ($('port')             || {value:''}).value;
  const host     = ($('host')             || {value:''}).value;
  const dbPath   = ($('db_path')          || {value:''}).value;
  const apiKey   = ($('anthropic_api_key')|| {value:''}).value;
  const ollamaU  = ($('ollama_base_url')  || {value:''}).value;
  const timeout  = ($('ollama_timeout')   || {value:''}).value;

  const maskedKey = apiKey ? apiKey.substring(0, 12) + '...' : 'empty';
  log('restartApp', 'new config: port=' + port2 + ' model=' + model + ' key=' + maskedKey);

  const fd = new FormData();
  fd.append('port',              port2);
  fd.append('host',              host);
  fd.append('db_path',           dbPath);
  fd.append('anthropic_api_key', apiKey);
  fd.append('ollama_base_url',   ollamaU);
  fd.append('ollama_model',      model);
  fd.append('ollama_timeout',    timeout);
  const modeElR = document.querySelector('input[name="analysis_mode"]:checked');
  fd.append('analysis_mode', modeElR ? modeElR.value : 'standard');

  try {
    log('restartApp', 'POST /restart');
    const res  = await fetch('/restart', { method: 'POST', body: fd });
    log('restartApp', 'response status=' + res.status);

    if (!res.ok) {
      const text = await res.text();
      logErr('restartApp', 'server error ' + res.status + ': ' + text);
      if (btn) { btn.disabled = false; btn.innerHTML = '↺ &nbsp;Restart'; }
      return;
    }

    const data = await res.json();
    log('restartApp', 'response:', data);

    if (data.ok) {
      currentAppUrl = data.url;
      log('restartApp', 'restarted at: ' + currentAppUrl);
      setRunningState(currentAppUrl);
      // Brief delay then open new URL
      setTimeout(() => window.open(currentAppUrl, '_blank'), 1000);
    } else {
      logErr('restartApp', 'server returned ok=false:', data);
      if (btn) { btn.disabled = false; btn.innerHTML = '↺ &nbsp;Restart'; }
    }
  } catch(e) {
    logErr('restartApp', 'fetch threw:', e);
    if (btn) { btn.disabled = false; btn.innerHTML = '↺ &nbsp;Restart'; }
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
  const startBtn     = $('start-btn');
  const runningPanel = $('running-panel');
  const urlText      = $('url-text');
  const urlLink      = $('url-link');
  const restartBtn   = $('restart-btn');
  const stopBtn      = $('stop-btn');
  const restartNote  = $('restart-note');

  if (startBtn) {
    startBtn.innerHTML = '✓ &nbsp;Running';
    startBtn.style.background = 'var(--green)';
    startBtn.style.color = '#fff';
    startBtn.style.borderColor = 'var(--green)';
    startBtn.disabled = true;
  }
  if (urlText)      urlText.textContent = url;
  if (urlLink)      urlLink.href = url;
  if (runningPanel) runningPanel.classList.remove('hidden');
  if (restartBtn)   { restartBtn.disabled = false; restartBtn.innerHTML = '↺ &nbsp;Restart'; }
  if (stopBtn)      { stopBtn.disabled = false; stopBtn.innerHTML = '■ &nbsp;Stop'; }
  if (restartNote)  restartNote.classList.remove('hidden');
}

function setStoppedState() {
  log('setStoppedState', 'resetting to start state');
  currentAppUrl = '';
  const startBtn     = $('start-btn');
  const runningPanel = $('running-panel');
  const restartNote  = $('restart-note');

  if (startBtn) {
    startBtn.innerHTML = '▶ &nbsp;Start Job Matcher';
    startBtn.style.background = '';
    startBtn.style.color = '';
    startBtn.style.borderColor = '';
    startBtn.disabled = false;
  }
  if (runningPanel) runningPanel.classList.add('hidden');
  if (restartNote)  restartNote.classList.add('hidden');
}

// ── Init ──────────────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', () => {
  log('init', 'DOMContentLoaded fired');
  runHealthChecks();
});
</script>
</body>
</html>`,
		cfg.Port,
		cfg.Host,
		cfg.DBPath,
		cfg.AnthropicAPIKey,
		cfg.OllamaBaseURL,
		cfg.OllamaModel,
		cfg.OllamaModel,
		cfg.OllamaTimeoutSeconds,
		checkedIf(cfg.AnalysisMode == "fast"),
		checkedIf(cfg.AnalysisMode == "standard" || cfg.AnalysisMode == ""),
		checkedIf(cfg.AnalysisMode == "detailed"),
	)
}
