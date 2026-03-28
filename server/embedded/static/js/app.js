// =============================================== //
// == Script loaded ============================== //
// =============================================== //
console.log("[app.js] Script loaded v6");

// =============================================== //
// == Logging helper ============================= //
// =============================================== //
function log(fn, msg, ...args) {
  console.log(`[${fn}]`, msg, ...args);
}
function logErr(fn, msg, ...args) {
  console.error(`[${fn}] ERROR:`, msg, ...args);
}

// =============================================== //
// == Toast ====================================== //
// =============================================== //
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

// =============================================== //
// == Tabs ======================================= //
// =============================================== //
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

// =============================================== //
// == Score meter ================================ //
// =============================================== //
function renderMeter(score, container) {
  container.innerHTML = "";
  for (let i = 1; i <= 5; i++) {
    const pip = document.createElement("div");
    pip.className = "score-pip" + (i <= score ? ` filled-${score}` : "");
    container.appendChild(pip);
  }
}

// =============================================== //
// == Add Mode Toggle ============================ //
// =============================================== //
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

// =============================================== //
// == Add Job by URL ============================= //
// =============================================== //
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
    log("addJob", "POST /api/jobs/add");
    const res  = await fetch("/api/jobs/add", { method: "POST", body: fd });
    const data = await res.json();
    log("addJob", `response status=${res.status}`, data);

    if (res.status === 409) {
      toast("Job already added — " + (data.title || url), "info");
      btn.disabled = false; btn.textContent = "Add Job";
      return;
    }
    if (!res.ok) {
      logErr("addJob", `server error ${res.status}:`, data.error);
      toast(data.error || "Failed to scrape URL", "error");
      btn.disabled = false; btn.textContent = "Add Job";
      return;
    }
    log("addJob", `success id=${data.job_id} title="${data.title}"`);
    toast(`✓ Added: ${data.title || url}`, "success");
    input.value = "";
    btn.disabled = false; btn.textContent = "Add Job";
    setTimeout(() => location.reload(), 800);
  } catch(err) {
    logErr("addJob", "fetch threw:", err);
    toast("Network error", "error");
    btn.disabled = false; btn.textContent = "Add Job";
  }
}

// =============================================== //
// == Add Job by Paste =========================== //
// =============================================== //
async function addJobManual() {
  const titleEl   = document.getElementById("paste-title");
  const companyEl = document.getElementById("paste-company");
  const descEl    = document.getElementById("paste-description");
  const btn       = document.getElementById("paste-submit-btn");

  if (!descEl) { logErr("addJobManual", "#paste-description not found"); return; }

  const title       = titleEl   ? titleEl.value.trim()   : "";
  const company     = companyEl ? companyEl.value.trim() : "";
  const description = descEl.value.trim();

  log("addJobManual", `title="${title}" company="${company}" desc_len=${description.length}`);

  if (!description) { toast("Please paste a job description", "error"); return; }
  if (description.length < 50) { toast("Description too short (min 50 chars)", "error"); return; }

  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span> Saving…';

  const fd = new FormData();
  fd.append("title",       title);
  fd.append("company",     company);
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
    if (titleEl)   titleEl.value   = "";
    if (companyEl) companyEl.value = "";
    descEl.value = "";
    btn.disabled = false; btn.textContent = "Add Job";
    setTimeout(() => location.reload(), 800);
  } catch(err) {
    logErr("addJobManual", "fetch threw:", err);
    toast("Network error", "error");
    btn.disabled = false; btn.textContent = "Add Job";
  }
}

// =============================================== //
// == Analyze Job ================================ //
// =============================================== //
// =============================================== //
// == Progress Bar =============================== //
// =============================================== //

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

async function analyzeJob(jobId) {
  const resumeSelect  = document.getElementById('analyze-resume');
  const providerInput = document.querySelector('input[name="provider"]:checked');
  const btn           = document.getElementById('analyze-btn');

  if (!resumeSelect) { logErr('analyzeJob', '#analyze-resume not found'); return; }
  if (!resumeSelect.value) { toast('Please select a resume first', 'error'); return; }

  const provider = providerInput ? providerInput.value : 'anthropic';
  const mode     = btn ? (btn.dataset.mode || 'standard') : 'standard';

  // Build model label for progress bar
  const ollamaLabel = document.querySelector('label[for="p-ollama"]');
  const model = provider === 'ollama'
    ? (ollamaLabel ? ollamaLabel.textContent.trim() : 'Ollama')
    : 'claude-opus-4-5';

  log('analyzeJob', `jobId=${jobId} resumeId=${resumeSelect.value} provider=${provider} mode=${mode}`);

  btn.disabled    = true;
  btn.textContent = 'Analyzing…';
  startProgress(provider, model, mode);

  const fd = new FormData();
  fd.append('resume_id', resumeSelect.value);
  fd.append('provider',  provider);

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

// =============================================== //
// == Save Application =========================== //
// =============================================== //
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
      logErr("saveApplication", `server error ${res.status}:`, data.error);
      toast(data.error || "Save failed", "error");
    }
  } catch(err) {
    logErr("saveApplication", "fetch threw:", err);
    toast("Network error", "error");
  }
  btn.disabled = false; btn.innerHTML = "Save";
}

// =============================================== //
// == Delete Analysis ============================ //
// =============================================== //
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
      logErr("deleteAnalysis", `server returned ${res.status}`);
      toast("Delete failed", "error");
    }
  } catch(err) {
    logErr("deleteAnalysis", "fetch threw:", err);
    toast("Network error", "error");
  }
}

// =============================================== //
// == Delete Job ================================= //
// =============================================== //
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
      logErr("deleteJob", `server returned ${res.status}`);
      toast("Delete failed", "error");
    }
  } catch(err) {
    logErr("deleteJob", "fetch threw:", err);
    toast("Network error", "error");
  }
}

// =============================================== //
// == Delete Resume ============================== //
// =============================================== //
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
      logErr("deleteResume", `server returned ${res.status}`);
      toast("Delete failed", "error");
    }
  } catch(err) {
    logErr("deleteResume", "fetch threw:", err);
    toast("Network error", "error");
  }
}

// =============================================== //
// == Add Resume ================================= //
// =============================================== //
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

// =============================================== //
// == Toggle description ========================= //
// =============================================== //
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
        logErr("toggleDesc", "fetch threw:", err);
        box.textContent = "Failed to load description.";
      }
    }
    box.classList.remove("hidden");
  } else {
    box.classList.add("hidden");
  }
}

// =============================================== //
// == Init ======================================= //
// =============================================== //
document.addEventListener("DOMContentLoaded", () => {
  log("init", "DOMContentLoaded fired");

  initTabs();
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


// =============================================== //
// == Search, Filter & Pagination ================ //
// =============================================== //

let _currentPage = 1;
let _perPage     = 25;
let _searchTimer = null;

// =============================================== //
// == Fetch jobs from server ===================== //
// =============================================== //
async function fetchJobs() {
  const search   = (document.getElementById('filter-search')   || {value:''}).value.trim();
  const status   = (document.getElementById('filter-status')   || {value:''}).value;
  const score    = (document.getElementById('filter-score')    || {value:''}).value;
  const provider = (document.getElementById('filter-provider') || {value:''}).value;

  const params = new URLSearchParams({
    page:     _currentPage,
    per_page: _perPage,
  });
  if (search)   params.set('search',   search);
  if (status)   params.set('status',   status);
  if (score)    params.set('score',    score);
  if (provider) params.set('provider', provider);

  // Sync URL bar (bookmarkable, back button works)
  const newURL = window.location.pathname + (params.toString() ? '?' + params.toString() : '');
  history.pushState({}, '', newURL);

  // Show/hide clear button
  const clearBtn = document.getElementById('clear-btn');
  if (clearBtn) clearBtn.style.display = (search || status || score || provider) ? 'inline-flex' : 'none';

  log('fetchJobs', `page=${_currentPage} per_page=${_perPage} search=${search} status=${status} score=${score} provider=${provider}`);

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

// =============================================== //
// == Render jobs list from API response ========= //
// =============================================== //
function renderJobs(data) {
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
    const providerTag = isManual
      ? `<span class="provider-tag">manual</span>`
      : (job.provider ? `<span class="provider-tag">${job.provider}</span>` : '');

    const status = job.status || 'not_applied';
    const meta   = (job.company || '') +
                   (job.company && job.location ? ' · ' : '') +
                   (job.location || '') ||
                   (isManual ? 'pasted description' : (job.url || '').substring(0, 60) + '…');

    return `<a href="/job/${job.id}" class="job-item" style="text-decoration:none;">
      <div>${scoreBadge}</div>
      <div class="job-item-info">
        <div class="job-title">${job.title || 'Untitled Job'}</div>
        <div class="job-meta">${meta}</div>
      </div>
      <div class="job-item-right">
        ${providerTag}
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

// =============================================== //
// == Render pagination controls ================= //
// =============================================== //
function renderPagination(data) {
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

// =============================================== //
// == Helpers ==================================== //
// =============================================== //
function showLoading(show) {
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
  return ['filter-search','filter-status','filter-score','filter-provider']
    .some(id => { const el = document.getElementById(id); return el && el.value !== ''; });
}

// =============================================== //
// == User actions =============================== //
// =============================================== //
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
  ['filter-search','filter-status','filter-score','filter-provider'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.value = '';
  });
  applyFilters();
  log('clearFilters', 'all filters cleared');
}

// =============================================== //
// == Restore state from URL params ============== //
// =============================================== //
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

  const hasFilters = params.get('search') || params.get('status') ||
                     params.get('score')  || params.get('provider');
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

  setEl('filter-search',   getValue('search'));
  setEl('filter-status',   getValue('status'));
  setEl('filter-score',    getValue('score'));
  setEl('filter-provider', getValue('provider'));

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

// =============================================== //
// == Init ======================================= //
// =============================================== //
document.addEventListener('DOMContentLoaded', () => {
  const jobsList = document.getElementById('jobs-list');
  if (!jobsList) return; // not on jobs page

  // Bind search input with debounce
  const searchEl = document.getElementById('filter-search');
  if (searchEl) searchEl.addEventListener('input', applyFiltersDebounced);

  // Bind dropdowns — instant
  ['filter-status','filter-score','filter-provider'].forEach(id => {
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


// =============================================== //
// == Snippet Evidence Toggle ==================== //
// =============================================== //

function toggleSnippets(btn) {
  const container = btn.nextElementSibling;
  const isHidden = container.style.display === 'none' || !container.style.display;
  container.style.display = isHidden ? 'block' : 'none';
  btn.innerHTML = isHidden ? '&#9660; evidence' : '&#9658; evidence';
}
