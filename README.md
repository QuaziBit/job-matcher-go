# Job Matcher — Go Version

> Local AI-powered resume-to-job matching and application tracker.
> Single binary — no Python, no venv, no pip install.
> Double-click the `.exe` (Windows) or run `./job-matcher-linux` (Linux).

---

## Screenshots

<div align="center">
  <img src="screenshots/01-launcher.jpg" width="350"/>
  <p><em>Launcher — live health checks, model selector, Start / Restart / Stop</em></p>
</div>

<div align="center">
  <img src="screenshots/05-job-tracker.jpg" width="800"/>
  <p><em>Job Tracker — jobs list with score badges and status tags</em></p>
</div>

<div align="center">
  <img src="screenshots/03-analysis-ollama-2.jpg" width="800"/>
  <p><em>Ollama (gemma3:27b) — analysis tab with resume selector and LLM provider toggle</em></p>
</div>

<div align="center">
  <img src="screenshots/03-analysis-ollama-1.jpg" width="800"/>
  <p><em>Ollama (llama3.1:8b) — same job, lighter penalty, different model perspective</em></p>
</div>

<div align="center">
  <img src="screenshots/02-analysis-anthropic.jpg" width="800"/>
  <p><em>Anthropic (claude-opus-4-5) — penalty pipeline with severity-weighted gaps</em></p>
</div>

---

## What It Does

- Scrapes job postings from a URL or accepts pasted/manual job descriptions
- Compares each job description against your resume using an LLM
- Scores the match from **1** (poor) to **5** (excellent) with a skill breakdown
- Applies a **penalty pipeline** — detects hard blockers like clearance requirements and experience minimums and adjusts the raw score automatically
- Estimates salary range for each job using the configured LLM provider
- Vets companies by crawling BBB, Glassdoor, and LinkedIn, then running an LLM legitimacy assessment
- Validates recruiter email domains via MX DNS lookup — no personal data leaves the machine
- Tracks application status, recruiter contact info, and personal notes
- Supports **Anthropic**, **OpenAI**, **Gemini**, and **Ollama** (local models, fully offline)

- HTML, CSS, and JS are embedded inside the binary — no external files needed at runtime
  _(known issue: Google Fonts are currently loaded from Google CDN — will be bundled locally in a future update)_

**Stack:** Go 1.21 · net/http · SQLite (pure Go) · html/template · go:embed

---

## Requirements

- Go 1.21 or higher → https://go.dev/dl/
- Internet access on first build (to download dependencies)
- Ollama (optional) → https://ollama.com
- Anthropic API key (optional) → https://console.anthropic.com

---

### PowerShell Permission Fix (Windows only)

If you get an execution policy error in PowerShell, run this first:

```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
```

---

## First-Time Setup

### Step 1 — Clone

```bash
git clone https://github.com/QuaziBit/job-matcher-go.git
cd job-matcher-go
```

---

### Step 2 — Download dependencies

```bash
go mod tidy
```

This downloads two packages:

| Package | Purpose |
|---|---|
| `modernc.org/sqlite` | Pure Go SQLite driver — no CGo, no C compiler needed |
| `golang.org/x/net` | HTML parser for job scraping |

---

### Step 3 — Run tests

```bash
go test ./...
# or
make test
```

Expected output:

```
ok  github.com/QuaziBit/job-matcher-go/config     0.4s
ok  github.com/QuaziBit/job-matcher-go/launcher   0.9s
ok  github.com/QuaziBit/job-matcher-go/server     0.6s
```

Or use the pretty runner:

```bash
go run ./cmd/testrunner
# or
make test-pretty
```

```
========================================================
  Job Matcher — Test Suite
========================================================

config
  [✓] TestDefaults
  [✓] TestSaveAndLoad
  [✓] TestLoadCreatesDefaultsWhenMissing
  ...

launcher
  [✓] TestCheckSQLite_ExistingFile
  [✓] TestCheckOllama_ServerRunning
  [✓] TestLauncherStartPostSendsConfig
  ...

server
  [✓] TestParseLLMResponse_ValidJSON
  [✓] TestComputeAdjustedScore_BlockerReducesScore
  [✓] TestDBJob_InsertAndFetch
  ...

--------------------------------------------------------
  70/70 passed
```

---

### Step 4 — Run the app

```bash
go run .
# or
make run
```

A browser window opens automatically with the **Launcher** — a config page where you can:

- Review health status of SQLite, Ollama, and Anthropic API
- Change the port, model, or API key before starting
- Click **▶ Start Job Matcher** to launch the main app
- Use **↺ Restart** to switch models without restarting manually
- Use **■ Stop** to shut down the server

Config is saved to `cfg/config.json` in your project directory.

---

## Building Binaries

```bash
make build-windows   # → dist/job-matcher.exe      (Windows amd64)
make build-linux     # → dist/job-matcher-linux    (Linux amd64)
make build-all       # → both
```

On Windows without `make`:

```powershell
go build -ldflags="-s -w" -o dist\job-matcher.exe .
```

The binary is fully self-contained — templates, CSS, and JS are embedded inside it via `go:embed`. No external files are needed to run it.

---

## Usage

### 1. Add your resume

Go to the **Resumes** page → paste your full resume text → give it a label.

```
Example labels: "DevSecOps v2", "Full-Stack General", "Federal Contractor"
```

You can store multiple versions and pick which one to compare per job.

### 2. Add a job

**From a URL** — paste any job posting URL and click Add Job. The app scrapes and parses it automatically.

**By pasting** — switch to the Paste tab, paste the description directly. Useful for Workday or login-gated pages that block scraping.

### 3. Run an analysis

Open a job → **Analysis tab** → pick a resume + LLM provider → **Run Analysis**.

You get:

- A **1–5 raw score** from the LLM
- A **penalty-adjusted score** accounting for hard blockers (clearance, years of experience, mandatory certs)
- **Matched skills** — what aligns
- **Missing skills** — each rated `blocker / major / minor`
- A short **reasoning** summary

Run multiple analyses with different resumes or providers and compare them side by side.

### 4. Track your application

**Application tab** → set status, add recruiter info, and write notes.

| Status | Meaning |
|---|---|
| `not_applied` | Saved for later |
| `applied` | Submitted |
| `interviewing` | In process |
| `offered` | Offer received |
| `rejected` | Closed |

---

## LLM Providers

### Anthropic (default)

Requires `anthropic_api_key` set in the launcher or `cfg/config.json`. Uses `claude-opus-4-5`.
Costs roughly $0.01–0.05 per analysis depending on job description length.
Get a key at: https://console.anthropic.com

### Ollama (local, free, offline)

Runs entirely on your machine — no API key, no cost, no data leaves your box.

```bash
# 1. Install Ollama
#    https://ollama.com/download

# 2. Pull a model
ollama pull llama3.1:8b

# 3. Start the server
ollama serve

# 4. Select the model in the launcher dropdown and click Start
```

**Recommended models:**

| Model | RAM needed | Quality |
|---|---|---|
| `llama3.1:8b` | ~8 GB | Best balance for everyday use |
| `gemma4:e4b` | ~8 GB | Strong quality, good JSON reliability |
| `gemma3:27b` | ~32 GB | Near-Anthropic quality |
| `phi3.5:3.8b` | ~4 GB | Fast triage only |

> ⚠️ `mistral` family models are not recommended — known to produce false skill gap detection and unreliable structured output.

**Recommended workflow:**

- Quick triage → `llama3.1:8b` or `gemma4:e4b`
- Serious roles → `gemma3:27b`
- Final decision → Anthropic or Gemini or OpenAI

---

## Scraping Notes

Works with most public job boards that serve static HTML. Tested with LinkedIn and Indeed. Other boards may work but have not been fully verified.

May not work if the page:

- Requires a login to view the full description
- Renders content entirely via JavaScript (some Workday / iCIMS pages)
- Has aggressive bot detection

If the scraper cannot extract a company name from the page, the job will be grouped under **Unknown Company** on the vetting page. Use the job detail page to set the company name manually.

**Workaround:** Use the **Paste** tab — copy the job description text from your browser and paste it directly. Or use the **Manual** tab to enter the job details by hand without a URL.

## Company Vetting Notes

The vetting page crawls three public sources for each company:

- **BBB** (Better Business Bureau) — rating and listing status
- **Glassdoor** — rating and review count
- **LinkedIn** — employee count and founded date

Crawling may return partial results depending on bot detection and page availability. Results are cached for 7 days.

---

## Releasing on GitHub

Push a version tag to trigger the release workflow:

```bash
git tag v1.0.0
git push origin v1.0.0
```

GitHub Actions will automatically:

1. Run all tests
2. Build `job-matcher.exe` (Windows amd64)
3. Build `job-matcher-linux` (Linux amd64)
4. Attach both to the GitHub release as downloadable assets

---

## Project Structure

```
job-matcher-go/
├── main.go                         # Entry point — starts launcher and HTTP server
├── go.mod                          # Go module dependencies
├── Makefile                        # Build, run, and test targets
├── config.example.json             # Example launcher config file
├── .cursorignore                   # Excludes secrets/keys from Cursor AI context
│
├── config/                         # Configuration package
│   ├── config.go                   # Config struct, env var loading, JSON parsing
│   └── config_test.go              # Config tests
│
├── launcher/                       # GUI launcher (system tray + config form)
│   ├── launcher.go                 # Launcher logic and HTTP config server
│   ├── template.go                 # Launcher HTML template rendering
│   ├── health.go                   # Health check for launcher
│   ├── launcher_test.go            # Launcher tests
│   └── health_test.go              # Health check tests
│
├── server/                         # Core application server
│   ├── server.go                   # HTTP server setup and route registration
│   ├── handlers.go                 # All API endpoint handlers
│   ├── database.go                 # SQLite schema, migrations, and DB helpers
│   ├── models.go                   # Request/response structs
│   ├── llm.go                      # Core LLM call (Anthropic/OpenAI/Gemini/Ollama)
│   ├── salary.go                   # Salary estimation via LLM
│   ├── company_vetter.go           # LLM company legitimacy vetting (no PII)
│   ├── gemini.go                   # Gemini AFC helper (no-op for REST API)
│   ├── company_crawler.go          # Company data crawler (BBB, Glassdoor, LinkedIn)
│   ├── mx_validator.go             # Email domain MX validation via nslookup (no PII)
│   ├── scraper.go                  # Job URL scraper (HTML → title/company/description)
│   ├── prompts.go                  # Prompt templates for job analysis
│   ├── parsers.go                  # LLM response parsers
│   ├── penalties.go                # Score penalty logic
│   ├── skills.go                   # Skills extraction utilities
│   ├── known_models.go             # Known model definitions per provider
│   ├── analyzer_config.go          # Analyzer config (models, URLs, env vars)
│   ├── extract.go                  # Resume/document text extraction
│   ├── utils.go                    # Shared utilities
│   ├── analyzer_test.go            # LLM analysis tests
│   ├── company_crawler_test.go     # Crawler tests
│   ├── company_vetter_test.go      # Company vetting tests
│   ├── database_test.go            # DB schema, migration, and helper tests
│   ├── extract_test.go             # Extraction tests
│   ├── handlers_test.go            # API endpoint tests
│   ├── mx_validator_test.go        # MX validation tests
│   ├── prompts_test.go             # Prompt tests
│   ├── scraper_test.go             # Scraper tests
│   ├── skills_test.go              # Skills tests
│   └── testhelpers_test.go         # Shared test fixtures and helpers
│
├── assets/                         # Embedded frontend assets (via go:embed)
│   ├── assets.go                   # go:embed directive for UI files
│   └── ui/                         # Shared frontend (vanilla JS + HTML)
│       ├── index.html              # Jobs list page
│       ├── job_detail.html         # Job detail page
│       ├── job_preview.html        # Job preview page
│       ├── resumes.html            # Resumes page
│       ├── vetting.html            # Vetting page (companies + recruiters)
│       ├── static/
│       │   ├── js/
│       │   │   └── app.js          # Main frontend JS (~3000 lines)
│       │   └── css/
│       │       └── style.css       # Stylesheet
│       └── launcher/
│           ├── launcher.html       # Launcher config form
│           ├── launcher.js         # Launcher frontend logic
│           └── launcher.css        # Launcher styles
│
├── cmd/
│   └── testrunner/
│       └── main.go                 # Test runner entry point
│
└── tests_js/
    └── test_app.html               # Browser-based JS unit tests
```

---

## Useful Go Commands

### After cloning or changing module path

```bash
go mod tidy       # regenerates go.sum with the new module path
go test ./...     # verify everything still compiles and passes
go run .          # confirm it starts
```

### Cache

```bash
go clean -cache           # clear build cache (compiled packages)
go clean -modcache        # clear module download cache
go clean -cache -modcache # clear both at once
```

### Modules

```bash
go mod tidy               # add missing, remove unused dependencies
go mod download           # download all dependencies to local cache
go mod verify             # check cached deps match go.sum
go get package@version    # add or upgrade a dependency
go get package@none       # remove a dependency
```

### Building

```bash
go build .                                               # build for current OS
GOOS=windows GOARCH=amd64 go build -o dist/app.exe .   # cross-compile to Windows
GOOS=linux   GOARCH=amd64 go build -o dist/app-linux . # cross-compile to Linux
go build -ldflags="-s -w" -o dist/app.exe .             # strip debug info (smaller binary)
```

### Testing

```bash
go test ./...              # run all tests
go test ./... -v           # verbose output
go test ./... -count=1     # disable cache, always re-run
go run ./cmd/testrunner    # pretty [✓] / [X] output
make test                  # alias for go test ./... -count=1
make test-pretty           # alias for go run ./cmd/testrunner
```

### Environment

```bash
go env                     # print all Go environment variables
go env GOPATH              # where modules and cache are stored
go version                 # print installed Go version
go fmt ./...               # format all Go files
go vet ./...               # catch common mistakes
```

---

**Local model compatibility (Ollama):**

- `llama3.1:8b` — works reliably in all modes (fast / standard / detailed) and for company vetting
- `llama3.2:3b` — works in all modes (fast / standard / detailed)
- `phi3.5:3.8b` — works in all modes (fast / standard / detailed)
- `gemma3:27b` — works in fast and standard modes; detailed mode may take
  15-20+ minutes on consumer hardware, increase `OLLAMA_TIMEOUT` to 2000s+
- `gemma4:e4b` — works reliably in all modes and for company vetting
- `gemma4:e2b` — works reliably in all modes and for company vetting
- `mistral` family — **not recommended**, known to produce false skill gap
  detection and unreliable structured output
- `nemotron-3-nano` — not compatible, ignores structured output format

**Cloud model compatibility:**

- Anthropic (`claude-haiku-4-5`, `claude-sonnet-4-6`, `claude-opus-4-6`) — works reliably in all modes
- OpenAI (`gpt-4o-mini`, `gpt-4o`, `gpt-4-turbo`, `o1-mini`, `o1`) — works reliably in all modes
- Gemini (`gemini-2.5-flash`, `gemini-2.5-flash-lite`, `gemini-2.5-pro`, `gemini-2.0-flash`) — works reliably in all modes

---

## Related

- **Python version** → [github.com/QuaziBit/job-matcher-py](https://github.com/QuaziBit/job-matcher-py)
  Original prototype built with FastAPI. Requires Python 3.10+, pip, and a virtual environment.

---

## License

MIT
