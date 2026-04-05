// ── Location flags ────────────────────────────────────────────────────────────
const LOCATION_FLAGS = [
  { keywords: ['remote'],                           code: 'REMOTE' },
  { keywords: ['united states', ' us ', 'u.s.', 'usa', 'united states of america'], code: 'US' },
  { keywords: ['united kingdom', ' uk ', 'u.k.', 'england', 'scotland', 'wales'],   code: 'UK' },
  { keywords: ['canada', ' ca '],                   code: 'CA' },
  { keywords: ['germany', 'deutschland'],           code: 'DE' },
  { keywords: ['france'],                           code: 'FR' },
  { keywords: ['australia'],                        code: 'AU' },
  { keywords: ['india'],                            code: 'IN' },
  { keywords: ['netherlands', 'holland'],           code: 'NL' },
  { keywords: ['singapore'],                        code: 'SG' },
];

function getLocationFlag(location) {
  if (!location) return 'N/A';
  const lower = location.toLowerCase();
  // Purely numeric or very short — not a real location string
  if (/^\d+$/.test(lower.trim())) return 'N/A';
  for (const entry of LOCATION_FLAGS) {
    if (entry.keywords.some(kw => lower.includes(kw))) return entry.code;
  }
  return '';
}

// ── Logging helper ───────────────────────────────────────────────────────────
function log(fn, msg, ...args) {
  console.log(`[${fn}]`, msg, ...args);
}
function logErr(fn, msg, ...args) {
  console.error(`[${fn}] ERROR:`, msg, ...args);
}

// ── Toast ────────────────────────────────────────────────────────────────────
let toastTimer;
function toast(msg, type = "info") {
  log("toast", type, msg);
  const el = document.getElementById("toast");
  if (!el) { logErr("toast", "#toast element not found"); return; }
  el.textContent = msg;
  el.className = `show ${type}`;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.className = ""; }, 3500);
}

// ── Tabs ─────────────────────────────────────────────────────────────────────
function initTabs() {
  const bars = document.querySelectorAll(".tabs");
  log("initTabs", `found ${bars.length} tab bar(s)`);
  bars.forEach(tabBar => {
    tabBar.querySelectorAll(".tab").forEach(tab => {
      tab.addEventListener("click", () => {
        const target = tab.dataset.tab;
        log("tab:click", `switching to tab="${target}"`);
        const parent = tabBar.closest(".tab-container") || document;
        tabBar.querySelectorAll(".tab").forEach(t => t.classList.remove("active"));
        parent.querySelectorAll(".tab-content").forEach(c => c.classList.remove("active"));
        tab.classList.add("active");
        const content = parent.querySelector(`[data-tab-content="${target}"]`);
        if (content) {
          content.classList.add("active");
        } else {
          logErr("tab:click", `no content panel found for data-tab-content="${target}"`);
        }
      });
    });
  });
}

// activateTab programmatically clicks the tab with data-tab="tabName".
function activateTab(tabName) {
  const tab = document.querySelector(`.tab[data-tab="${tabName}"]`);
  if (tab) {
    log("activateTab", `activating tab="${tabName}"`);
    tab.click();
  }
}

// ── Score meter ──────────────────────────────────────────────────────────────
function renderMeter(score, container) {
  container.innerHTML = "";
  for (let i = 1; i <= 5; i++) {
    const pip = document.createElement("div");
    pip.className = "score-pip" + (i <= score ? ` filled-${score}` : "");
    container.appendChild(pip);
  }
}

// ── Add Mode Toggle ──────────────────────────────────────────────────────────
function initAddModeToggle() {
  const radios = document.querySelectorAll('input[name="add-mode"]');
  if (!radios.length) { log("initAddModeToggle", "no add-mode radios found — skipping"); return; }
  log("initAddModeToggle", `found ${radios.length} radio(s)`);
  radios.forEach(radio => {
    radio.addEventListener("change", () => {
      log("addModeToggle", `mode switched to "${radio.value}"`);
      const urlForm   = document.getElementById("add-job-form");
      const pasteForm = document.getElementById("paste-job-form-wrap");
      if (!urlForm)   { logErr("addModeToggle", "#add-job-form not found"); return; }
      if (!pasteForm) { logErr("addModeToggle", "#paste-job-form-wrap not found"); return; }
      if (radio.value === "paste" && radio.checked) {
        urlForm.classList.add("hidden");
        pasteForm.classList.remove("hidden");
      } else {
        urlForm.classList.remove("hidden");
        pasteForm.classList.add("hidden");
      }
    });
  });
}

// ── Add Job by URL ───────────────────────────────────────────────────────────
async function addJob(e) {
  e.preventDefault();
  const form  = e.target;
  const btn   = form.querySelector("[type=submit]");
  const input = form.querySelector("input[name=url]");

  if (!input) { logErr("addJob", "url input not found in form"); return; }
  const url = input.value.trim();
  log("addJob", `url="${url}"`);
  if (!url) { toast("Please enter a URL", "error"); return; }

  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span> Scraping…';

  const fd = new FormData();
  fd.append("url", url);

  try {
    log("addJob", "POST /api/jobs/scrape");
    const res  = await fetch("/api/jobs/scrape", { method: "POST", body: fd });
    const data = await res.json();
    log("addJob", `response status=${res.status}`, data);

    if (res.status === 409) {
      toast("Job already added — redirecting…", "info");
      btn.disabled = false; btn.textContent = "Add Job";
      setTimeout(() => { window.location.href = `/job/${data.job_id}`; }, 800);
      return;
    }
    if (!res.ok) {
      logErr("addJob", `server error ${res.status}:`, data.error);
      toast(data.error || "Failed to scrape URL", "error");
      btn.disabled = false; btn.textContent = "Add Job";
      return;
    }
    log("addJob", `scraped ok, storing preview and redirecting`);
    sessionStorage.setItem("job_preview", JSON.stringify({ url, ...data }));
    input.value = "";
    btn.disabled = false; btn.textContent = "Add Job";
    window.location.href = "/jobs/preview";
  } catch(err) {
    logErr("addJob", "fetch threw:", err);
    toast("Network error", "error");
    btn.disabled = false; btn.textContent = "Add Job";
  }
}

// ── Add Job by Paste ─────────────────────────────────────────────────────────
async function addJobManual() {
  const titleEl    = document.getElementById("paste-title");
  const companyEl  = document.getElementById("paste-company");
  const locationEl = document.getElementById("paste-location");
  const descEl     = document.getElementById("paste-description");
  const btn        = document.getElementById("paste-submit-btn");

  if (!descEl) { logErr("addJobManual", "#paste-description not found"); return; }

  const title       = titleEl    ? titleEl.value.trim()    : "";
  const company     = companyEl  ? companyEl.value.trim()  : "";
  const location    = locationEl ? locationEl.value.trim() : "";
  const description = descEl.value.trim();

  log("addJobManual", `title="${title}" company="${company}" location="${location}" desc_len=${description.length}`);

  if (!description) { toast("Please paste a job description", "error"); return; }
  if (description.length < 50) { toast("Description too short (min 50 chars)", "error"); return; }

  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span> Saving…';

  const fd = new FormData();
  fd.append("title",       title);
  fd.append("company",     company);
  fd.append("location",    location);
  fd.append("description", description);

  try {
    log("addJobManual", "POST /api/jobs/add-manual");
    const res  = await fetch("/api/jobs/add-manual", { method: "POST", body: fd });
    const data = await res.json();
    log("addJobManual", `response status=${res.status}`, data);

    if (res.status === 409) {
      toast("This description was already added", "info");
      btn.disabled = false; btn.textContent = "Add Job";
      return;
    }
    if (!res.ok) {
      logErr("addJobManual", `server error ${res.status}:`, data.error);
      toast(data.error || "Failed to save", "error");
      btn.disabled = false; btn.textContent = "Add Job";
      return;
    }
    log("addJobManual", `success id=${data.job_id} title="${data.title}"`);
    toast(`✓ Added: ${data.title}`, "success");
    btn.disabled = false; btn.textContent = "Add Job";
    setTimeout(() => { window.location.href = `/job/${data.job_id}`; }, 800);
  } catch(err) {
    logErr("addJobManual", "fetch threw:", err);
    toast("Network error", "error");
    btn.disabled = false; btn.textContent = "Add Job";
  }
}

// ── Analyze Job & Progress Bar ───────────────────────────────────────────────

const MODE_ESTIMATES = { fast: 30, standard: 90, detailed: 240 };
let _progressTimer = null;
let _progressStart = null;

function startProgress(provider, model, mode) {
  const el    = document.getElementById('analysis-progress');
  const label = document.getElementById('progress-label');
  const fill  = document.getElementById('progress-fill');
  const meta  = document.getElementById('progress-meta');
  const est   = MODE_ESTIMATES[mode] || 90;
  if (el) el.classList.remove('hidden');
  _progressStart = Date.now();
  if (label) label.textContent = `Analyzing with ${model} · ${mode} mode`;
  if (fill)  fill.style.width = '0%';
  _progressTimer = setInterval(() => {
    const elapsed = (Date.now() - _progressStart) / 1000;
    const pct     = Math.min(elapsed / est * 100, 95);
    if (fill) fill.style.width = pct + '%';
    if (meta) meta.textContent = `${formatElapsed(elapsed)} elapsed · ~${formatElapsed(est)} estimated`;
  }, 500);
  log('startProgress', `provider=${provider} model=${model} mode=${mode} est=${est}s`);
}

function stopProgress() {
  clearInterval(_progressTimer);
  _progressTimer = null;
  const fill = document.getElementById('progress-fill');
  if (fill) fill.style.width = '100%';
  setTimeout(() => {
    const el = document.getElementById('analysis-progress');
    if (el) el.classList.add('hidden');
  }, 400);
}

function formatElapsed(seconds) {
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return m > 0 ? `${m}:${s.toString().padStart(2, '0')}` : `${s}s`;
}

// ── Model population ──────────────────────────────────────────────────────────
async function populateOllamaModels() {
  const sel = document.getElementById('analyze-ollama-model');
  if (!sel) return;
  const defaultVal = sel.dataset.default || sel.value;
  try {
    const res  = await fetch('/api/ollama/models');
    if (!res.ok) return;
    const data = await res.json();
    const models = data.models || [];
    if (models.length === 0) return;
    sel.innerHTML = '';
    models.forEach(m => {
      const opt = document.createElement('option');
      opt.value = m; opt.textContent = m;
      if (m === defaultVal) opt.selected = true;
      sel.appendChild(opt);
    });
    if (!models.includes(defaultVal) && defaultVal) {
      const opt = document.createElement('option');
      opt.value = defaultVal; opt.textContent = defaultVal + ' (not found)';
      opt.selected = true;
      sel.insertBefore(opt, sel.firstChild);
    }
  } catch(e) {
    logErr('populateOllamaModels', 'fetch threw:', e);
  }
}

async function populateCloudModels(provider) {
  const sel = document.getElementById('analyze-cloud-model');
  if (!sel) return;
  const defaultVal = sel.dataset[provider] || sel.value;
  try {
    const res  = await fetch(`/api/providers/models?provider=${provider}`);
    if (!res.ok) return;
    const data = await res.json();
    const models = data.models || [];
    if (models.length === 0) return;
    sel.innerHTML = '';
    models.forEach(m => {
      const opt = document.createElement('option');
      opt.value = m.id; opt.textContent = m.label;
      if (m.id === defaultVal) opt.selected = true;
      sel.appendChild(opt);
    });
  } catch(e) {
    logErr('populateCloudModels', 'fetch threw:', e);
  }
}

function updateProviderModelRow() {
  const providerEl = document.querySelector('input[name="provider"]:checked');
  const provider   = providerEl ? providerEl.value : 'anthropic';
  const ollamaRow  = document.getElementById('model-row-ollama');
  const cloudRow   = document.getElementById('model-row-cloud');
  if (!ollamaRow || !cloudRow) return;
  if (provider === 'ollama') {
    ollamaRow.classList.remove('hidden');
    cloudRow.classList.add('hidden');
  } else {
    cloudRow.classList.remove('hidden');
    ollamaRow.classList.add('hidden');
    populateCloudModels(provider);
  }
  log('updateProviderModelRow', `provider=${provider}`);
}

async function analyzeJob(jobId) {
  const resumeSelect  = document.getElementById('analyze-resume');
  const providerInput = document.querySelector('input[name="provider"]:checked');
  const modeSelect    = document.getElementById('analyze-mode');
  const btn           = document.getElementById('analyze-btn');

  if (!resumeSelect) { logErr('analyzeJob', '#analyze-resume not found'); return; }
  if (!resumeSelect.value) { toast('Please select a resume first', 'error'); return; }

  const provider = providerInput ? providerInput.value : 'anthropic';
  const mode     = modeSelect ? modeSelect.value : 'standard';

  // Resolve per-request model
  let ollamaModel = '', cloudModel = '';
  if (provider === 'ollama') {
    const sel = document.getElementById('analyze-ollama-model');
    ollamaModel = sel ? sel.value : '';
  } else {
    const sel = document.getElementById('analyze-cloud-model');
    cloudModel = sel ? sel.value : '';
  }
  const modelLabel = provider === 'ollama' ? (ollamaModel || 'Ollama') : (cloudModel || 'Anthropic');

  log('analyzeJob', `jobId=${jobId} resumeId=${resumeSelect.value} provider=${provider} mode=${mode} model=${modelLabel}`);

  btn.disabled    = true;
  btn.textContent = 'Analyzing…';
  startProgress(provider, modelLabel, mode);

  const fd = new FormData();
  fd.append('resume_id',     resumeSelect.value);
  fd.append('provider',      provider);
  fd.append('analysis_mode', mode);
  if (ollamaModel) fd.append('ollama_model', ollamaModel);
  if (cloudModel)  fd.append('cloud_model',  cloudModel);

  try {
    log('analyzeJob', `POST /api/jobs/${jobId}/analyze`);
    const res  = await fetch(`/api/jobs/${jobId}/analyze`, { method: 'POST', body: fd });
    const data = await res.json();
    log('analyzeJob', `response status=${res.status}`, data);
    stopProgress();

    if (!res.ok) {
      logErr('analyzeJob', `server error ${res.status}:`, data.error);
      toast(data.error || 'Analysis failed', 'error');
      btn.disabled = false; btn.textContent = 'Run Analysis';
      return;
    }
    log('analyzeJob', `success score=${data.score} adjusted=${data.adjusted_score}`);
    toast(`✓ Score: ${data.adjusted_score}/5 (raw ${data.score}/5)`, 'success');
    btn.disabled = false; btn.textContent = 'Run Analysis';
    setTimeout(() => location.reload(), 800);
  } catch(err) {
    stopProgress();
    logErr('analyzeJob', 'fetch threw:', err);
    toast('Network error', 'error');
    btn.disabled = false; btn.textContent = 'Run Analysis';
  }
}

// ── Save Application ─────────────────────────────────────────────────────────
async function saveApplication(jobId) {
  const statusEl = document.getElementById("app-status");
  const nameEl   = document.getElementById("recruiter-name");
  const emailEl  = document.getElementById("recruiter-email");
  const phoneEl  = document.getElementById("recruiter-phone");
  const notesEl  = document.getElementById("app-notes");
  const btn      = document.getElementById("save-app-btn");

  if (!statusEl) { logErr("saveApplication", "#app-status not found"); return; }

  const status = statusEl.value;
  log("saveApplication", `jobId=${jobId} status="${status}"`);

  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span>';

  const fd = new FormData();
  fd.append("status",          status);
  fd.append("recruiter_name",  nameEl  ? nameEl.value  : "");
  fd.append("recruiter_email", emailEl ? emailEl.value : "");
  fd.append("recruiter_phone", phoneEl ? phoneEl.value : "");
  fd.append("notes",           notesEl ? notesEl.value : "");

  try {
    log("saveApplication", `POST /api/jobs/${jobId}/application`);
    const res  = await fetch(`/api/jobs/${jobId}/application`, { method: "POST", body: fd });
    const data = await res.json();
    log("saveApplication", `response status=${res.status}`, data);

    if (res.ok) {
      toast("Application info saved", "success");
      const badge = document.getElementById("status-badge");
      if (badge) {
        badge.className = `status-badge status-${status}`;
        badge.textContent = status.replace("_", " ");
      }
    } else {
      logErr("saveApplication", `server error ${res.status} job=${jobId}:`, data.error);
      toast(data.error || "Save failed", "error");
    }
  } catch(err) {
    logErr("saveApplication", "fetch threw:", err);
    toast("Network error", "error");
  }
  btn.disabled = false; btn.innerHTML = "Save";
}

// ── Delete Analysis ──────────────────────────────────────────────────────────
async function deleteAnalysis(analysisId) {
  if (!confirm("Remove this analysis?")) return;
  log("deleteAnalysis", `id=${analysisId}`);
  try {
    const res = await fetch(`/api/analyses/${analysisId}`, { method: "DELETE" });
    log("deleteAnalysis", `response status=${res.status}`);
    if (res.ok) {
      const block = document.getElementById(`analysis-${analysisId}`);
      if (block) block.remove(); else logErr("deleteAnalysis", `#analysis-${analysisId} not found in DOM`);
      toast("Analysis removed", "info");
    } else {
      logErr("deleteAnalysis", `server returned ${res.status} id=${analysisId}`);
      toast("Delete failed", "error");
    }
  } catch(err) {
    logErr("deleteAnalysis", "fetch threw:", err);
    toast("Network error", "error");
  }
}

// ── Delete Job ───────────────────────────────────────────────────────────────
async function deleteJob(jobId) {
  if (!confirm("Delete this job and all its analyses?")) return;
  log("deleteJob", `id=${jobId}`);
  try {
    const res = await fetch(`/api/jobs/${jobId}`, { method: "DELETE" });
    log("deleteJob", `response status=${res.status}`);
    if (res.ok) {
      toast("Job deleted", "info");
      setTimeout(() => location.href = "/", 600);
    } else {
      logErr("deleteJob", `server returned ${res.status} id=${jobId}`);
      toast("Delete failed", "error");
    }
  } catch(err) {
    logErr("deleteJob", "fetch threw:", err);
    toast("Network error", "error");
  }
}

// ── Delete Resume ────────────────────────────────────────────────────────────
async function deleteResume(resumeId) {
  if (!confirm("Delete this resume version?")) return;
  log("deleteResume", `id=${resumeId}`);
  try {
    const res = await fetch(`/api/resumes/${resumeId}`, { method: "DELETE" });
    log("deleteResume", `response status=${res.status}`);
    if (res.ok) {
      toast("Resume deleted", "info");
      setTimeout(() => location.reload(), 600);
    } else {
      logErr("deleteResume", `server returned ${res.status} id=${resumeId}`);
      toast("Delete failed", "error");
    }
  } catch(err) {
    logErr("deleteResume", "fetch threw:", err);
    toast("Network error", "error");
  }
}

// ── Add Resume ───────────────────────────────────────────────────────────────
async function addResume(e) {
  if (e) e.preventDefault();

  const form = document.getElementById("add-resume-form");
  if (!form) { logErr("addResume", "form#add-resume-form not found"); return; }

  const fd      = new FormData(form);
  const label   = (fd.get("label")   || "").trim();
  const content = (fd.get("content") || "").trim();

  log("addResume", `label="${label}" content_len=${content.length}`);

  if (!label || !content) {
    logErr("addResume", `validation failed — label="${label}" content_len=${content.length}`);
    toast("Label and content are required", "error");
    return;
  }

  const btn = form.querySelector("[type=submit]");
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span>'; }

  try {
    log("addResume", "POST /api/resumes/add");
    const res  = await fetch("/api/resumes/add", { method: "POST", body: fd });
    const data = await res.json();
    log("addResume", `response status=${res.status}`, data);

    if (res.ok) {
      log("addResume", `success id=${data.resume_id} label="${data.label}"`);
      toast(`✓ Resume "${data.label}" saved`, "success");
      form.reset();
      if (btn) { btn.disabled = false; btn.textContent = "Save Resume"; }
      setTimeout(() => location.reload(), 800);
    } else {
      logErr("addResume", `server error ${res.status}:`, data.error);
      toast(data.error || "Failed", "error");
      if (btn) { btn.disabled = false; btn.textContent = "Save Resume"; }
    }
  } catch(err) {
    logErr("addResume", "fetch threw:", err);
    toast("Network error", "error");
    if (btn) { btn.disabled = false; btn.textContent = "Save Resume"; }
  }
}

// ── Toggle description ───────────────────────────────────────────────────────
async function toggleDesc(jobId) {
  const box = document.getElementById("desc-box");
  if (!box) { logErr("toggleDesc", "#desc-box not found"); return; }

  if (box.classList.contains("hidden")) {
    if (!box.dataset.loaded) {
      log("toggleDesc", `fetching description for job=${jobId}`);
      box.textContent = "Loading…";
      try {
        const res  = await fetch(`/api/jobs/${jobId}/description`);
        const data = await res.json();
        log("toggleDesc", `loaded ${data.description?.length || 0} chars`);
        box.textContent = data.description || "(no description)";
        box.dataset.loaded = "1";
      } catch(err) {
        logErr("toggleDesc", `jobId=${jobId} fetch threw:`, err);
        box.textContent = "Failed to load description.";
      }
    }
    box.classList.remove("hidden");
  } else {
    box.classList.add("hidden");
  }
}

// ── Init ─────────────────────────────────────────────────────────────────────
document.addEventListener("DOMContentLoaded", () => {
  log("init", "DOMContentLoaded fired");

  initTabs();

  // Activate tab from URL hash (e.g. /job/1#application)
  if (window.location.hash) {
    const hashTab = window.location.hash.slice(1);
    log("init", `hash tab activation: "${hashTab}"`);
    activateTab(hashTab);
  }

  initAddModeToggle();

  const addJobForm = document.getElementById("add-job-form");
  if (addJobForm) {
    log("init", "binding #add-job-form submit");
    addJobForm.addEventListener("submit", addJob);
  } else {
    log("init", "#add-job-form not on this page");
  }

  const addResumeForm = document.getElementById("add-resume-form");
  if (addResumeForm) {
    log("init", "binding #add-resume-form submit");
    addResumeForm.addEventListener("submit", addResume);
  } else {
    log("init", "#add-resume-form not on this page");
  }

  // Job detail: provider/model/mode dropdowns
  if (document.getElementById('analyze-mode')) {
    updateProviderModelRow();
    populateOllamaModels();
    document.querySelectorAll('input[name="provider"]').forEach(el => {
      el.addEventListener('change', updateProviderModelRow);
    });
    log('init', 'job detail model dropdowns initialized');
  }

  // Render score meters on job detail page
  document.querySelectorAll("[id^='meter-']").forEach(el => {
    const block  = el.closest(".analysis-block");
    if (!block) return;
    const badges = block.querySelectorAll(".score-badge");
    const badge  = badges.length > 1 ? badges[1] : badges[0];
    if (!badge) return;
    const score = parseInt(badge.textContent.trim());
    if (!isNaN(score)) {
      log("init", `rendering meter score=${score}`);
      renderMeter(score, el);
    }
  });
});


// ── Search, Filter & Pagination ──────────────────────────────────────────────

// ── Jobs list state ───────────────────────────────────────────────────────────
let _currentPage = 1;
let _perPage     = 25;
let _searchTimer = null;

// ── Fetch jobs from server ────────────────────────────────────────────────────
async function fetchJobs() {
  const search     = (document.getElementById('filter-search')    || {value:''}).value.trim();
  const status     = (document.getElementById('filter-status')    || {value:''}).value;
  const score      = (document.getElementById('filter-score')     || {value:''}).value;
  const provider   = (document.getElementById('filter-provider')  || {value:''}).value;
  const addedDays  = (document.getElementById('filter-date')      || {value:''}).value;
  const dateFrom   = (document.getElementById('filter-date-from') || {value:''}).value;
  const dateTo     = (document.getElementById('filter-date-to')   || {value:''}).value;

  const params = new URLSearchParams({
    page:     _currentPage,
    per_page: _perPage,
  });
  if (search)    params.set('search',     search);
  if (status)    params.set('status',     status);
  if (score)     params.set('score',      score);
  if (provider)  params.set('provider',   provider);
  if (addedDays) params.set('added_days', addedDays);
  if (dateFrom)  params.set('date_from',  dateFrom);
  if (dateTo)    params.set('date_to',    dateTo);

  // Sync URL bar (bookmarkable, back button works)
  const newURL = window.location.pathname + (params.toString() ? '?' + params.toString() : '');
  history.pushState({}, '', newURL);

  // Show/hide clear button
  const clearBtn = document.getElementById('clear-btn');
  if (clearBtn) clearBtn.style.display =
    (search || status || score || provider || addedDays || dateFrom || dateTo) ? 'inline-flex' : 'none';

  log('fetchJobs', `page=${_currentPage} per_page=${_perPage} search=${search} status=${status} score=${score} provider=${provider} added_days=${addedDays} date_from=${dateFrom} date_to=${dateTo}`);

  showLoading(true);

  try {
    const res = await fetch('/api/jobs/list?' + params);

    // Handle non-200 responses with server error message
    if (!res.ok) {
      let errMsg = `Server error ${res.status}`;
      try {
        const errData = await res.json();
        errMsg = errData.error || errMsg;
      } catch(_) {}
      logErr('fetchJobs', `HTTP ${res.status}: ${errMsg}`);
      showError(`Failed to load jobs: ${errMsg}`);
      return;
    }

    let data;
    try {
      data = await res.json();
    } catch(parseErr) {
      logErr('fetchJobs', 'failed to parse JSON response:', parseErr);
      showError('Failed to load jobs: server returned invalid data.');
      return;
    }

    // Validate expected fields
    if (typeof data.total === 'undefined' || !Array.isArray(data.jobs)) {
      logErr('fetchJobs', 'unexpected response shape:', data);
      showError('Failed to load jobs: unexpected response from server.');
      return;
    }

    log('fetchJobs', `total=${data.total} pages=${data.total_pages}`);
    renderJobs(data);

  } catch(err) {
    // Network failure — server unreachable
    logErr('fetchJobs', 'fetch threw:', err);
    showError('Could not reach the server. Is Job Matcher still running?');
  }
}

// ── Render jobs list ─────────────────────────────────────────────────────────
function renderJobs(data) {
  log('renderJobs', `total=${data.total} jobs=${data.jobs.length}`);
  showLoading(false);

  const list       = document.getElementById('jobs-list');
  const noResults  = document.getElementById('no-results');
  const emptyState = document.getElementById('empty-state');
  const pagBar     = document.getElementById('pagination-bar');

  if (!list) return;

  // No jobs in DB at all
  if (data.total === 0 && !hasActiveFilter()) {
    list.innerHTML = '';
    if (noResults)  noResults.classList.add('hidden');
    if (emptyState) emptyState.classList.remove('hidden');
    if (pagBar)     pagBar.style.display = 'none';
    return;
  }

  // Filters active but no results
  if (data.jobs.length === 0) {
    list.innerHTML = '';
    if (noResults)  noResults.classList.remove('hidden');
    if (emptyState) emptyState.classList.add('hidden');
    if (pagBar)     pagBar.style.display = 'none';
    return;
  }

  if (noResults)  noResults.classList.add('hidden');
  if (emptyState) emptyState.classList.add('hidden');

  // Build job items HTML
  list.innerHTML = data.jobs.map(job => {
    if (!job || typeof job.id === 'undefined') {
      logErr('renderJobs', 'malformed job item:', job);
      return ''; // skip malformed items silently
    }
    const score   = job.adjusted_score || job.best_score;
    const scoreBadge = score
      ? `<div class="score-badge score-${score}">${score}</div>`
      : `<div class="score-badge score-none">—</div>`;

    const isManual = job.url && job.url.startsWith('manual://');

    const sourceTag = isManual
      ? `<span class="provider-tag">MANUAL</span>`
      : `<span class="provider-tag">SCRAPED</span>`;

    const modelTag = job.provider && job.last_model
      ? `<span class="provider-tag" style="max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${job.provider} · ${job.last_model}">${job.provider} · ${job.last_model}</span>`
      : '';

    const recruiterBadge = job.has_recruiter
      ? `<span class="provider-tag" style="cursor:pointer;" onclick="event.preventDefault();window.location='/job/${job.id}#application';" title="Recruiter contact saved">👤</span>`
      : '';

    const locCode  = getLocationFlag(job.location || '');
    const locBadge = locCode ? `<span class="location-code">${locCode}</span>` : '';

    const status = job.status || 'not_applied';
    const metaText = (job.company || '') +
                     (job.company && job.location ? ' · ' : '') +
                     (job.location || '') ||
                     (isManual ? 'pasted description' : (job.url || '').substring(0, 60) + '…');

    const dateLabel = formatJobDate(job.scraped_at);
    const dateTag   = dateLabel ? ` <span class="date-tag">added ${dateLabel}</span>` : '';

    return `<a href="/job/${job.id}" class="job-item" style="text-decoration:none;">
      <div>${scoreBadge}</div>
      <div class="job-item-info">
        <div class="job-title">${job.title || 'Untitled Job'}</div>
        <div class="job-meta">${metaText}${locBadge ? ' ' + locBadge : ''}${dateTag}</div>
      </div>
      <div class="job-item-right">
        ${recruiterBadge}
        ${modelTag || sourceTag}
        <span class="status-badge status-${status}">${status}</span>
      </div>
    </a>`;
  }).join('');

  // Update pagination bar
  renderPagination(data);

  // Save filter state when clicking a job — so Back restores it
  list.querySelectorAll('a.job-item').forEach(link => {
    link.addEventListener('click', () => {
      saveFilterState();
    });
  });
}

// ── Render pagination controls ───────────────────────────────────────────────
function renderPagination(data) {
  log('renderPagination', `page=${data.page}/${data.total_pages} total=${data.total}`);
  const pagBar   = document.getElementById('pagination-bar');
  const info     = document.getElementById('pagination-info');
  const indicator = document.getElementById('page-indicator');
  const prevBtn  = document.getElementById('prev-btn');
  const nextBtn  = document.getElementById('next-btn');

  if (!pagBar) return;

  const perPage    = data.per_page === 0 ? data.total : data.per_page;
  const start      = ((data.page - 1) * perPage) + 1;
  const end        = Math.min(data.page * perPage, data.total);
  const totalPages = data.total_pages;

  pagBar.style.display = data.total > 0 ? 'flex' : 'none';

  if (info)      info.textContent = `Showing ${start}–${end} of ${data.total} job${data.total !== 1 ? 's' : ''}`;
  if (indicator) indicator.textContent = totalPages > 1 ? `Page ${data.page} of ${totalPages}` : '';
  if (prevBtn)   prevBtn.disabled = data.page <= 1;
  if (nextBtn)   nextBtn.disabled = data.page >= totalPages;

  // Hide pagination bar if everything fits
  if (totalPages <= 1 && _perPage !== 0) {
      pagBar.style.display = 'none';
  }
}

// ── Helpers ──────────────────────────────────────────────────────────────────
function showLoading(show) {
  log('showLoading', 'show=' + show);
  const loading = document.getElementById('jobs-loading');
  const list    = document.getElementById('jobs-list');
  const errBox  = document.getElementById('jobs-error');
  if (loading) loading.style.display = show ? 'block' : 'none';
  if (errBox)  errBox.classList.add('hidden');
  if (list && show) list.innerHTML = '';
}

function showError(msg) {
  showLoading(false);
  const errBox  = document.getElementById('jobs-error');
  const errText = document.getElementById('jobs-error-text');
  const list    = document.getElementById('jobs-list');
  const pagBar  = document.getElementById('pagination-bar');
  const noRes   = document.getElementById('no-results');
  const empty   = document.getElementById('empty-state');

  if (list)    list.innerHTML = '';
  if (pagBar)  pagBar.style.display = 'none';
  if (noRes)   noRes.classList.add('hidden');
  if (empty)   empty.classList.add('hidden');

  if (errBox && errText) {
    errText.textContent = msg;
    errBox.classList.remove('hidden');
  } else {
    // Fallback to toast if error box not in DOM
    toast(msg, 'error');
  }
  logErr('showError', msg);
}

function hasActiveFilter() {
  return ['filter-search','filter-status','filter-score','filter-provider','filter-date','filter-date-from','filter-date-to']
    .some(id => { const el = document.getElementById(id); return el && el.value !== ''; });
}

// ── User actions ─────────────────────────────────────────────────────────────
function applyFilters() {
  _currentPage = 1;
  fetchJobs();
}

function applyFiltersDebounced() {
  clearTimeout(_searchTimer);
  _searchTimer = setTimeout(applyFilters, 300);
}

function changePage(dir) {
  _currentPage += dir;
  fetchJobs();
}

function changePerPage() {
  const sel = document.getElementById('per-page');
  _perPage = sel ? parseInt(sel.value) : 25;
  _currentPage = 1;
  fetchJobs();
  log('changePerPage', `perPage=${_perPage}`);
}

function clearFilters() {
  ['filter-search','filter-status','filter-score','filter-provider','filter-date','filter-date-from','filter-date-to'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.value = '';
  });
  updateSearchClearBtn();
  applyFilters();
  log('clearFilters', 'all filters cleared');
}

function clearSearch() {
  const el = document.getElementById('filter-search');
  if (el) el.value = '';
  updateSearchClearBtn();
  applyFilters();
  log('clearSearch', 'search cleared');
}

// ── Date mode toggle (simple dropdown ↔ from/to pickers) ─────────────────────
let _dateMode = 'simple';
function toggleDateMode() {
  const simpleEl = document.getElementById('filter-date');
  const fromEl   = document.getElementById('filter-date-from');
  const toEl     = document.getElementById('filter-date-to');
  const modeBtn  = document.getElementById('date-mode-btn');

  _dateMode = _dateMode === 'simple' ? 'advanced' : 'simple';
  log('toggleDateMode', 'mode=' + _dateMode);

  if (_dateMode === 'advanced') {
    if (simpleEl) { simpleEl.value = ''; simpleEl.style.display = 'none'; }
    if (fromEl)   fromEl.style.display   = 'inline-block';
    if (toEl)     toEl.style.display     = 'inline-block';
    if (modeBtn)  modeBtn.textContent    = '✕ range';
  } else {
    if (fromEl)  { fromEl.value = '';   fromEl.style.display  = 'none'; }
    if (toEl)    { toEl.value   = '';   toEl.style.display    = 'none'; }
    if (simpleEl) simpleEl.style.display = '';
    if (modeBtn)  modeBtn.textContent    = '⋯';
    applyFilters();
  }
}

// ── Date formatting for job cards ─────────────────────────────────────────────
function formatJobDate(dateStr) {
  if (!dateStr) return '';
  const d    = new Date(dateStr);
  if (isNaN(d)) return '';
  const now  = new Date();
  const months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
  const month  = months[d.getMonth()];
  const day    = d.getDate();
  if (d.getFullYear() === now.getFullYear()) {
    return `${month} ${day}`;
  }
  return `${month} ${day}, ${d.getFullYear()}`;
}

function updateSearchClearBtn() {
  const el  = document.getElementById('filter-search');
  const btn = document.getElementById('search-clear-btn');
  if (btn) btn.style.display = (el && el.value) ? 'block' : 'none';
}

// ── Restore state from URL params ────────────────────────────────────────────
function saveFilterState() {
  const state = {
    search:   (document.getElementById('filter-search')   || {value:''}).value,
    status:   (document.getElementById('filter-status')   || {value:''}).value,
    score:    (document.getElementById('filter-score')    || {value:''}).value,
    provider: (document.getElementById('filter-provider') || {value:''}).value,
    page:     _currentPage,
    per_page: _perPage,
  };
  try {
    sessionStorage.setItem('jobFilterState', JSON.stringify(state));
    log('saveFilterState', 'saved:', JSON.stringify(state));
  } catch(e) {
    logErr('saveFilterState', 'sessionStorage write failed:', e);
  }
}

function restoreFromURL() {
  const rawSearch = window.location.search;
  log('restoreFromURL', 'URL search string:', rawSearch);

  const params = new URLSearchParams(rawSearch);

  const hasFilters = params.get('search')     || params.get('status') ||
                     params.get('score')      || params.get('provider') ||
                     params.get('added_days') || params.get('date_from') || params.get('date_to');
  const hasPageParams = params.get('page') || params.get('per_page');

  log('restoreFromURL', 'hasFilters=' + hasFilters + ' hasPageParams=' + hasPageParams);

  let saved = null;
  if (!hasFilters) {
    try {
      const raw = sessionStorage.getItem('jobFilterState');
      if (raw) {
        saved = JSON.parse(raw);
        log('restoreFromURL', 'found sessionStorage state:', JSON.stringify(saved));
      }
    } catch(e) {
      logErr('restoreFromURL', 'sessionStorage read failed:', e);
    }
  }

  const getValue = (key) => {
    const fromURL = params.get(key);
    if (fromURL !== null) return fromURL;
    return saved ? (saved[key] || '') : '';
  };

  const setEl = (id, val) => {
    const el = document.getElementById(id);
    if (!el) { logErr('restoreFromURL', 'element not found: #' + id); return; }
    el.value = val || '';
    log('restoreFromURL', id + ' = ' + JSON.stringify(el.value));
  };

  setEl('filter-search',    getValue('search'));
  setEl('filter-status',    getValue('status'));
  setEl('filter-score',     getValue('score'));
  setEl('filter-provider',  getValue('provider'));
  setEl('filter-date',      getValue('added_days'));
  const fromVal = getValue('date_from');
  const toVal   = getValue('date_to');
  setEl('filter-date-from', fromVal);
  setEl('filter-date-to',   toVal);
  if (fromVal || toVal) {
    // Restore advanced mode if range params exist
    _dateMode = 'advanced';
    const simpleEl  = document.getElementById('filter-date');
    const fromEl    = document.getElementById('filter-date-from');
    const toEl      = document.getElementById('filter-date-to');
    const modeBtn   = document.getElementById('date-mode-btn');
    if (simpleEl) simpleEl.style.display = 'none';
    if (fromEl)   fromEl.style.display   = 'inline-block';
    if (toEl)     toEl.style.display     = 'inline-block';
    if (modeBtn)  modeBtn.textContent    = '✕ range';
  }

  if (hasPageParams) {
    _currentPage = parseInt(params.get('page')) || 1;
    const pp = params.get('per_page');
    _perPage = (pp !== null && pp !== '') ? parseInt(pp) : 25;
  } else if (saved) {
    _currentPage = saved.page     || 1;
    _perPage     = (saved.per_page !== undefined && saved.per_page !== null) ? saved.per_page : 25;
  }

  const perPageSel = document.getElementById('per-page');
  if (perPageSel) perPageSel.value = String(_perPage);

  log('restoreFromURL', 'final: page=' + _currentPage + ' perPage=' + _perPage);
}

// ── Init (jobs page) ─────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', () => {
  const jobsList = document.getElementById('jobs-list');
  if (!jobsList) return; // not on jobs page

  // Bind search input with debounce + clear button visibility
  const searchEl = document.getElementById('filter-search');
  if (searchEl) {
    searchEl.addEventListener('input', () => {
      applyFiltersDebounced();
      updateSearchClearBtn();
    });
  }

  // Bind dropdowns — instant
  ['filter-status','filter-score','filter-provider','filter-date'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.addEventListener('change', applyFilters);
  });

  // Bind date range pickers — instant
  ['filter-date-from','filter-date-to'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.addEventListener('change', applyFilters);
  });

  // Handle browser back/forward
  window.addEventListener('popstate', () => {
    restoreFromURL();
    fetchJobs();
  });

  // Restore filters from URL and load first page
  restoreFromURL();
  fetchJobs();
});


// ── Snippet Evidence Toggle ───────────────────────────────────────────────────

function toggleSnippets(btn) {
  const container = btn.nextElementSibling;
  if (!container) return;
  const isHidden = container.style.display === 'none' || !container.style.display;
  container.style.display = isHidden ? 'block' : 'none';
  btn.innerHTML = isHidden ? '&#9660; evidence' : '&#9658; evidence';
}

// ── Salary Estimation ────────────────────────────────────────────────────────

async function estimateSalary(jobId) {
  const btn = document.getElementById('salary-btn');
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> Estimating…'; }
  const providerEl = document.querySelector('input[name="provider"]:checked');
  const fd = new FormData();
  if (providerEl) fd.append('provider', providerEl.value);
  try {
    const res  = await fetch(`/api/jobs/${jobId}/estimate-salary`, { method: 'POST', body: fd });
    const data = await res.json();
    if (!res.ok) {
      toast(data.error || 'Failed to estimate salary', 'error');
      if (btn) { btn.disabled = false; btn.textContent = '💰 Estimate Salary'; }
      return;
    }
    toast('✓ Salary estimated', 'success');
    setTimeout(() => location.reload(), 800);
  } catch(err) {
    logErr('estimateSalary', 'fetch threw:', err);
    toast('Network error', 'error');
    if (btn) { btn.disabled = false; btn.textContent = '💰 Estimate Salary'; }
  }
}

async function clearSalaryEstimate(jobId) {
  try {
    const res = await fetch(`/api/jobs/${jobId}/salary-estimate`, { method: 'DELETE' });
    if (!res.ok) {
      const data = await res.json();
      logErr('clearSalaryEstimate', `server error ${res.status}:`, data.error);
      toast(data.error || 'Failed to clear salary', 'error');
      return;
    }
    setTimeout(() => location.reload(), 400);
  } catch(err) {
    logErr('clearSalaryEstimate', 'fetch threw:', err);
    toast('Network error', 'error');
  }
}

async function rerunSalaryEstimate(jobId) {
  const rerunBtn = document.querySelector(`button[onclick="rerunSalaryEstimate(${jobId})"]`);
  if (rerunBtn) { rerunBtn.disabled = true; rerunBtn.innerHTML = '<span class="spinner"></span> re-running…'; }
  try {
    const delRes = await fetch(`/api/jobs/${jobId}/salary-estimate`, { method: 'DELETE' });
    if (!delRes.ok) {
      const d = await delRes.json();
      logErr('rerunSalaryEstimate', 'DELETE error:', d.error);
      toast(d.error || 'Failed to clear salary', 'error');
      if (rerunBtn) { rerunBtn.disabled = false; rerunBtn.textContent = 're-run'; }
      return;
    }
    const providerEl = document.querySelector('input[name="provider"]:checked');
    const fd = new FormData();
    if (providerEl) fd.append('provider', providerEl.value);
    const estRes = await fetch(`/api/jobs/${jobId}/estimate-salary`, { method: 'POST', body: fd });
    const data   = await estRes.json();
    if (!estRes.ok) {
      logErr('rerunSalaryEstimate', 'POST error:', data.error);
      toast(data.error || 'Failed to estimate salary', 'error');
      if (rerunBtn) { rerunBtn.disabled = false; rerunBtn.textContent = 're-run'; }
      return;
    }
    toast('✓ Salary re-estimated', 'success');
    setTimeout(() => location.reload(), 800);
  } catch(err) {
    logErr('rerunSalaryEstimate', 'fetch threw:', err);
    toast('Network error', 'error');
    if (rerunBtn) { rerunBtn.disabled = false; rerunBtn.textContent = 're-run'; }
  }
}

// ── Job Preview Page ──────────────────────────────────────────────────────────

function updatePreviewCharCount() {
  const el = document.getElementById('preview-description');
  const counter = document.getElementById('preview-char-count');
  if (!el || !counter) return;
  counter.textContent = `(${el.value.length.toLocaleString()} chars)`;
}

async function savePreview() {
  const titleEl    = document.getElementById('preview-title');
  const companyEl  = document.getElementById('preview-company');
  const locationEl = document.getElementById('preview-location');
  const descEl     = document.getElementById('preview-description');
  if (!descEl) return;

  const preview = JSON.parse(sessionStorage.getItem('job_preview') || '{}');
  const url = preview.url || '';
  if (!url) { toast('Preview data missing — please try again', 'error'); return; }

  const description = descEl.value.trim();
  if (description.length < 50) { toast('Description too short (min 50 chars)', 'error'); return; }

  const btn = document.querySelector('#preview-save-btn') ||
              document.querySelector('button[onclick="savePreview()"]');
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> Saving…'; }

  const fd = new FormData();
  fd.append('url',         url);
  fd.append('title',       titleEl    ? titleEl.value.trim()    : '');
  fd.append('company',     companyEl  ? companyEl.value.trim()  : '');
  fd.append('location',    locationEl ? locationEl.value.trim() : '');
  fd.append('description', description);

  try {
    const res  = await fetch('/api/jobs/save-preview', { method: 'POST', body: fd });
    const data = await res.json();
    if (res.status === 409) {
      toast('Already added — redirecting…', 'info');
      sessionStorage.removeItem('job_preview');
      setTimeout(() => { window.location.href = `/job/${data.job_id}`; }, 800);
      return;
    }
    if (!res.ok) {
      toast(data.error || 'Failed to save', 'error');
      if (btn) { btn.disabled = false; btn.textContent = 'Save Job'; }
      return;
    }
    toast(`✓ Saved: ${data.title}`, 'success');
    sessionStorage.removeItem('job_preview');
    setTimeout(() => { window.location.href = `/job/${data.job_id}`; }, 800);
  } catch(err) {
    logErr('savePreview', 'fetch threw:', err);
    toast('Network error', 'error');
    if (btn) { btn.disabled = false; btn.textContent = 'Save Job'; }
  }
}

document.addEventListener('DOMContentLoaded', () => {
  if (!document.getElementById('preview-title')) return; // not on preview page

  const raw = sessionStorage.getItem('job_preview');
  if (!raw) { window.location.href = '/'; return; }

  const data = JSON.parse(raw);
  const set = (id, val) => { const el = document.getElementById(id); if (el) el.value = val || ''; };
  set('preview-title',       data.title);
  set('preview-company',     data.company);
  set('preview-location',    data.location);
  set('preview-description', data.description);
  updatePreviewCharCount();

  // Warnings
  const warningsEl  = document.getElementById('preview-warnings');
  const blockersEl  = document.getElementById('preview-blockers');
  const blockerList = document.getElementById('preview-blocker-list');
  const qualityEl   = document.getElementById('preview-quality');
  const qualLevel   = document.getElementById('preview-quality-level');
  const qualIssues  = document.getElementById('preview-quality-issues');

  if (data.has_warnings && warningsEl) {
    warningsEl.style.display = '';

    if (data.blocker_keywords && data.blocker_keywords.length > 0 && blockersEl) {
      blockersEl.style.display = '';
      blockerList.textContent = data.blocker_keywords.join(', ');
    }

    const tq = data.text_quality || {};
    if (tq.issues && tq.issues.length > 0 && qualityEl) {
      qualityEl.style.display = '';
      qualLevel.textContent = tq.level || 'warn';
      qualIssues.innerHTML = tq.issues.map(i => `<li>${i}</li>`).join('');
    }
  }
});
