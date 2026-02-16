package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/velocli/velocli/velocli-backend/internal/store"
)

type server struct {
	store *store.Store
}

const serverLatestVersion = "0.1.0"
const serverMinSupportedVersion = serverLatestVersion

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/v1/version", s.handleGetVersion)
	mux.HandleFunc("GET /api/v1/catalog", s.handleGetCatalog)
	mux.HandleFunc("GET /api/v1/catalog/stream", s.handleCatalogStream)
	mux.HandleFunc("PUT /api/v1/admin/catalog", s.handlePutCatalog)
	mux.HandleFunc("POST /api/v1/admin/blocks", s.handleUploadBlock)
	mux.HandleFunc("PUT /api/v1/admin/blocks/{id}", s.handleUpdateBlockMeta)
	mux.HandleFunc("DELETE /api/v1/admin/blocks/{id}", s.handleDeleteBlock)
	mux.HandleFunc("GET /api/v1/blocks/{id}/download", s.handleDownloadBlock)
	mux.HandleFunc("GET /admin", s.handleAdmin)
	mux.HandleFunc("GET /", s.handleRoot)
	return versionMiddleware(mux)
}

func (s *server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": "Not Found"})
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("OK"))
}

func (s *server) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"latestVersion":       serverLatestVersion,
		"minSupportedVersion": serverMinSupportedVersion,
	})
}

func (s *server) handleGetCatalog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.store.GetCatalog())
}

func (s *server) handleCatalogStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	_, ch, cancel := s.store.SubscribeCatalogChanges()
	defer cancel()

	lastVer := s.store.CatalogVersion()
	writeEvent := func(ver int64) {
		_, _ = w.Write([]byte("event: catalog\n"))
		_, _ = w.Write([]byte("data: " + fmtInt64(ver) + "\n\n"))
		flusher.Flush()
	}

	writeEvent(lastVer)

	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			lastVer = s.store.CatalogVersion()
			writeEvent(lastVer)
		case <-keepAlive.C:
			_, _ = w.Write([]byte("event: ping\n"))
			_, _ = w.Write([]byte("data: ok\n\n"))
			flusher.Flush()
		}
	}
}

func (s *server) handlePutCatalog(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var c store.Catalog
	if err := json.Unmarshal(body, &c); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.store.PutCatalog(c); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *server) handleUploadBlock(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	meta := r.FormValue("meta")
	if meta == "" {
		http.Error(w, "missing meta", http.StatusBadRequest)
		return
	}

	var b store.Block
	if err := json.Unmarshal([]byte(meta), &b); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, 64<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	enc, err := s.store.Encrypt(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.store.UpsertBlock(b, enc); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "blockId": b.ID})
}

func (s *server) handleUpdateBlockMeta(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var b store.Block
	if err := json.Unmarshal(body, &b); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	b.ID = id

	if err := s.store.UpsertBlockMeta(b); err != nil {
		if err.Error() == "block not found" {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "blockId": b.ID})
}

func (s *server) handleDeleteBlock(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if err := s.store.DeleteBlock(id); err != nil {
		if err.Error() == "block not found" {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *server) handleDownloadBlock(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	data, _, err := s.store.GetEncryptedBlockBlob(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}

func (s *server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(adminHTML))
}

func main() {
	addr := envDefault("VELOCLI_BACKEND_ADDR", "0.0.0.0:9999")
	baseDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	st, err := store.New(baseDir, "VELOCLI_DATA_KEY")
	if err != nil {
		log.Fatal(err)
	}

	s := &server{store: st}

	log.Printf("backend listening on http://%s", addr)
	log.Printf("admin: http://%s/admin", addr)
	log.Printf("catalog: http://%s/api/v1/catalog", addr)

	if err := http.ListenAndServe(addr, logMiddleware(s.routes())); err != nil {
		log.Fatal(err)
	}
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func fmtInt64(v int64) string {
	return strconv.FormatInt(v, 10)
}

func versionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/admin" || r.URL.Path == "/" {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/version") {
			next.ServeHTTP(w, r)
			return
		}
		clientVer := strings.TrimSpace(r.Header.Get("X-VeloCLI-Version"))
		if clientVer == "" {
			next.ServeHTTP(w, r)
			return
		}
		if semverLess(clientVer, serverMinSupportedVersion) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUpgradeRequired)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":               "upgrade required",
				"minSupportedVersion": serverMinSupportedVersion,
				"latestVersion":       serverLatestVersion,
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func semverLess(a, b string) bool {
	aa := parseSemver(a)
	bb := parseSemver(b)
	if aa[0] != bb[0] {
		return aa[0] < bb[0]
	}
	if aa[1] != bb[1] {
		return aa[1] < bb[1]
	}
	return aa[2] < bb[2]
}

func parseSemver(v string) [3]int {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		out[i] = atoi(parts[i])
	}
	return out
}

func atoi(s string) int {
	s = strings.TrimSpace(s)
	n := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

const adminHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>VeloAdmin</title>
  <style>
    :root {
      --bg: #0A0A0A;
      --panel: #0F0F10;
      --panel2: #111113;
      --border: #2A2A2D;
      --border2: #333;
      --text: #EDEDED;
      --muted: #A0A0AA;
      --muted2: #7A7A85;
      --blue: #2D7DFF;
      --blue2: #1F5FE0;
      --good: #22c55e;
      --bad: #ef4444;
      --warn: #f59e0b;
      --code: "JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
      --ui: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial;
      --shadow: 0 18px 45px rgba(0,0,0,.45);
    }

    html, body { height: 100%; }
    body {
      margin: 0;
      font-family: var(--ui);
      background: radial-gradient(1200px 800px at 15% 0%, rgba(45,125,255,.18), transparent 55%),
                  radial-gradient(900px 600px at 90% 10%, rgba(34,197,94,.08), transparent 55%),
                  var(--bg);
      color: var(--text);
    }

    .app {
      display: grid;
      grid-template-columns: 260px 1fr;
      height: 100%;
    }

    .sidebar {
      border-right: 1px solid var(--border);
      background: linear-gradient(180deg, rgba(255,255,255,.02), rgba(255,255,255,0));
      padding: 18px 14px;
    }

    .brand {
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 10px 10px 14px 10px;
      border: 1px solid var(--border);
      border-radius: 14px;
      background: rgba(255,255,255,.02);
      box-shadow: var(--shadow);
      margin-bottom: 14px;
    }
    .dot {
      width: 12px;
      height: 12px;
      border-radius: 999px;
      background: var(--blue);
      box-shadow: 0 0 0 6px rgba(45,125,255,.12);
    }
    .brand h1 { font-size: 14px; margin: 0; letter-spacing: .6px; }
    .brand p { margin: 2px 0 0 0; font-size: 12px; color: var(--muted); }

    .nav {
      display: flex;
      flex-direction: column;
      gap: 8px;
      margin-top: 10px;
    }
    .nav button {
      width: 100%;
      text-align: left;
      padding: 10px 12px;
      border-radius: 12px;
      border: 1px solid var(--border);
      background: rgba(255,255,255,.02);
      color: var(--text);
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: space-between;
    }
    .nav button.active {
      border-color: rgba(45,125,255,.45);
      background: rgba(45,125,255,.10);
      box-shadow: 0 0 0 4px rgba(45,125,255,.08);
    }
    .nav small { color: var(--muted); font-size: 12px; }

    .main {
      display: grid;
      grid-template-rows: 64px 1fr;
    }

    .topbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 0 18px;
      border-bottom: 1px solid var(--border);
      background: rgba(10,10,10,.7);
      backdrop-filter: blur(10px);
    }
    .topbar .left {
      display: flex;
      align-items: center;
      gap: 12px;
    }
    .badge {
      font-size: 12px;
      color: var(--muted);
      border: 1px solid var(--border);
      background: rgba(255,255,255,.02);
      padding: 6px 10px;
      border-radius: 999px;
    }
    .statusDot {
      width: 9px;
      height: 9px;
      border-radius: 999px;
      background: var(--muted2);
      display: inline-block;
      margin-right: 8px;
    }

    .actions {
      display: flex;
      align-items: center;
      gap: 10px;
    }
    .btn {
      border: 1px solid var(--border);
      background: rgba(255,255,255,.02);
      color: var(--text);
      border-radius: 12px;
      padding: 10px 12px;
      cursor: pointer;
    }
    .btn.primary {
      background: linear-gradient(180deg, rgba(45,125,255,.95), rgba(31,95,224,.95));
      border-color: rgba(45,125,255,.8);
      box-shadow: 0 12px 30px rgba(45,125,255,.25);
    }
    .btn.danger {
      border-color: rgba(239,68,68,.55);
      background: rgba(239,68,68,.08);
    }
    .btn:disabled {
      opacity: .55;
      cursor: not-allowed;
    }

    .content {
      padding: 18px;
      overflow: auto;
    }

    .pageTitle {
      display: flex;
      align-items: flex-end;
      justify-content: space-between;
      gap: 12px;
      margin-bottom: 14px;
    }
    .pageTitle h2 { margin: 0; font-size: 16px; letter-spacing: .4px; }
    .pageTitle p { margin: 6px 0 0 0; color: var(--muted); font-size: 12px; }

    .grid2 {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 14px;
    }
    .grid3 {
      display: grid;
      grid-template-columns: 1fr 1fr 1fr;
      gap: 14px;
    }

    .card {
      border: 1px solid var(--border);
      background: rgba(255,255,255,.02);
      border-radius: 16px;
      padding: 14px;
      box-shadow: var(--shadow);
    }
    .cardHeader {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 12px;
    }
    .cardHeader h3 { margin: 0; font-size: 13px; }
    .subtle { color: var(--muted); font-size: 12px; }

    .table {
      width: 100%;
      border-collapse: collapse;
      overflow: hidden;
      border-radius: 12px;
      border: 1px solid var(--border);
    }
    .table th, .table td {
      padding: 10px 10px;
      border-bottom: 1px solid var(--border);
      font-size: 12px;
      vertical-align: top;
    }
    .table th {
      color: var(--muted);
      font-weight: 600;
      background: rgba(255,255,255,.03);
      text-align: left;
    }
    .table tr:last-child td { border-bottom: none; }

    input, select, textarea {
      width: 100%;
      box-sizing: border-box;
      background: rgba(0,0,0,.35);
      border: 1px solid var(--border2);
      border-radius: 12px;
      padding: 10px 10px;
      color: var(--text);
      outline: none;
    }
    textarea.code { font-family: var(--code); min-height: 140px; }
    input.mono { font-family: var(--code); }
    label { display: block; color: var(--muted); font-size: 12px; margin-bottom: 6px; }

    .row {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 12px;
      margin-bottom: 12px;
    }
    .row3 { grid-template-columns: 1fr 1fr 1fr; }
    .row1 { grid-template-columns: 1fr; }

    .list {
      display: flex;
      flex-direction: column;
      gap: 8px;
    }
    .item {
      border: 1px solid var(--border);
      background: rgba(255,255,255,.02);
      border-radius: 14px;
      padding: 10px;
      display: flex;
      align-items: center;
      justify-content: space-between;
      cursor: pointer;
    }
    .item.active {
      border-color: rgba(45,125,255,.45);
      background: rgba(45,125,255,.08);
    }
    .item .title { font-size: 12px; font-weight: 600; }
    .item .meta { font-size: 12px; color: var(--muted); font-family: var(--code); }

    .chips {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
    }
    .chip {
      border: 1px solid var(--border);
      background: rgba(255,255,255,.02);
      color: var(--muted);
      padding: 5px 9px;
      border-radius: 999px;
      font-size: 12px;
      font-family: var(--code);
    }

    .toast {
      position: fixed;
      right: 16px;
      bottom: 16px;
      display: none;
      border: 1px solid var(--border);
      background: rgba(20,20,22,.92);
      border-radius: 14px;
      padding: 12px 12px;
      box-shadow: var(--shadow);
      max-width: 460px;
      z-index: 50;
    }
    .toast .t { font-size: 12px; color: var(--muted); }
    .toast .m { margin-top: 6px; font-size: 12px; }
    .toast.ok { border-color: rgba(34,197,94,.45); }
    .toast.err { border-color: rgba(239,68,68,.55); }

    .modalBack {
      position: fixed;
      inset: 0;
      background: rgba(0,0,0,.55);
      display: none;
      align-items: center;
      justify-content: center;
      z-index: 60;
    }
    .modal {
      width: min(720px, calc(100vw - 24px));
      max-height: calc(100vh - 24px);
      overflow: auto;
      background: rgba(15,15,16,.96);
      border: 1px solid var(--border);
      border-radius: 18px;
      box-shadow: var(--shadow);
      padding: 14px;
    }
    .modalTop {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 10px;
    }
    .modalTop h3 { margin: 0; font-size: 13px; }
  </style>
</head>
<body>
  <div class="app">
    <aside class="sidebar">
      <div class="brand">
        <span class="dot"></span>
        <div>
          <h1>VeloAdmin</h1>
          <p>Visual template builder (v0)</p>
        </div>
      </div>
      <nav class="nav">
        <button id="nav-catalog" class="active"><span>Catalog</span><small id="nav-catalog-count">—</small></button>
        <button id="nav-blocks"><span>Blocks</span><small id="nav-blocks-count">—</small></button>
        <button id="nav-templates"><span>Templates</span><small id="nav-templates-count">—</small></button>
      </nav>
      <div style="margin-top: 14px" class="card">
        <div class="subtle">Backend</div>
        <div class="chips" style="margin-top: 10px">
          <span class="chip" id="chip-api">api: —</span>
          <span class="chip" id="chip-stream">stream: —</span>
        </div>
        <div style="margin-top: 10px" class="subtle">Tip: keep admin open while you test the CLI. Changes push live.</div>
      </div>
    </aside>

    <main class="main">
      <header class="topbar">
        <div class="left">
          <span class="badge"><span class="statusDot" id="statusDot"></span><span id="statusText">Connecting…</span></span>
          <span class="badge">Catalog v<span id="catalogVer">—</span></span>
        </div>
        <div class="actions">
          <button class="btn" id="btn-open-admin">Open in New Tab</button>
          <button class="btn" id="btn-reload">Reload</button>
          <button class="btn primary" id="btn-save-catalog">Save Catalog</button>
        </div>
      </header>

      <section class="content">
        <div id="view-catalog">
          <div class="pageTitle">
            <div>
              <h2>Catalog</h2>
              <p>Categories and selection rules that drive the CLI experience.</p>
            </div>
            <div class="actions">
              <button class="btn" id="btn-add-category">Add Category</button>
            </div>
          </div>
          <div class="card">
            <table class="table">
              <thead>
                <tr>
                  <th style="width: 36%">Name</th>
                  <th style="width: 24%">ID</th>
                  <th style="width: 22%">Selection</th>
                  <th style="width: 10%">Blocks</th>
                  <th style="width: 8%"></th>
                </tr>
              </thead>
              <tbody id="cat-table"></tbody>
            </table>
            <div class="subtle" style="margin-top: 10px">Deleting a category does not delete blocks; it only removes it from selection.</div>
          </div>
        </div>

        <div id="view-blocks" style="display:none">
          <div class="pageTitle">
            <div>
              <h2>Blocks</h2>
              <p>Reusable zipped code chunks + metadata. No JSON editing required.</p>
            </div>
            <div class="actions">
              <button class="btn" id="btn-new-block">New Block</button>
            </div>
          </div>

          <div class="grid2">
            <div class="card">
              <div class="cardHeader">
                <h3>Library</h3>
                <span class="subtle" id="block-count">—</span>
              </div>
              <div class="row row1" style="margin-bottom: 10px">
                <div>
                  <label>Search</label>
                  <input id="block-search" placeholder="Search by label or id" />
                </div>
              </div>
              <div class="row row1" style="margin-bottom: 10px">
                <div>
                  <label>Category filter</label>
                  <select id="block-category-filter"></select>
                </div>
              </div>
              <div class="list" id="block-list"></div>
            </div>

            <div class="card">
              <div class="cardHeader">
                <h3 id="block-editor-title">Block Editor</h3>
                <span class="subtle" id="block-editor-sub">—</span>
              </div>

              <div class="row row3">
                <div>
                  <label>Block ID</label>
                  <input id="b-id" class="mono" placeholder="Leave empty to auto-generate" />
                </div>
                <div>
                  <label>Label</label>
                  <input id="b-label" placeholder="Firebase Init" />
                </div>
                <div>
                  <label>Category</label>
                  <select id="b-category"></select>
                </div>
              </div>

              <div class="row row1">
                <div>
                  <label>Description</label>
                  <input id="b-desc" placeholder="What does this block do?" />
                </div>
              </div>

              <div class="row row3">
                <div>
                  <label>Base Path</label>
                  <input id="b-base" class="mono" placeholder="lib/" />
                </div>
                <div>
                  <label>Main Target</label>
                  <input id="b-main-target" class="mono" placeholder="lib/main.dart" />
                </div>
                <div>
                  <label>Main Mode</label>
                  <select id="b-main-mode">
                    <option value="append">append</option>
                    <option value="prepend">prepend</option>
                    <option value="inject">inject</option>
                    <option value="replace">replace</option>
                  </select>
                </div>
              </div>

              <div class="row row1">
                <div>
                  <label>Main Content (Dart). Use placeholder {{APP_TITLE}} if needed.</label>
                  <textarea id="b-main-content" class="code" spellcheck="false"></textarea>
                </div>
              </div>

              <div class="row">
                <div>
                  <label>Dependencies</label>
                  <div id="deps" class="list"></div>
                  <button class="btn" id="btn-add-dep" style="margin-top:10px">Add Dependency</button>
                </div>
                <div>
                  <label>Conflicts</label>
                  <div id="conflicts" class="list" style="max-height: 240px; overflow:auto; padding-right: 6px;"></div>
                  <div class="subtle" style="margin-top:10px">Conflicts hide incompatible options in the CLI.</div>
                </div>
              </div>

              <div class="row row1">
                <div>
                  <label>Zip file (required for new blocks; optional for updates)</label>
                  <input id="b-zip" type="file" accept=".zip,application/zip" />
                </div>
              </div>

              <div class="actions" style="justify-content: flex-end">
                <button class="btn danger" id="btn-delete-block" disabled>Delete</button>
                <button class="btn primary" id="btn-save-block">Save Block</button>
              </div>
            </div>
          </div>
        </div>

        <div id="view-templates" style="display:none">
          <div class="pageTitle">
            <div>
              <h2>Templates</h2>
              <p>Base main.dart templates shown in the CLI. Keep the content empty to use Flutter defaults.</p>
            </div>
            <div class="actions">
              <button class="btn" id="btn-add-template">Add Template</button>
            </div>
          </div>

          <div class="grid2" id="tpl-grid"></div>
          <div class="subtle" style="margin-top: 10px">Templates are saved via Save Catalog.</div>
        </div>
      </section>
    </main>
  </div>

  <div class="toast" id="toast">
    <div class="t" id="toastTitle">—</div>
    <div class="m" id="toastMsg">—</div>
  </div>

  <div class="modalBack" id="modalBack">
    <div class="modal">
      <div class="modalTop">
        <h3 id="modalTitle">—</h3>
        <button class="btn" id="modalClose">Close</button>
      </div>
      <div id="modalBody"></div>
      <div class="actions" style="justify-content:flex-end; margin-top: 12px">
        <button class="btn primary" id="modalPrimary">Save</button>
      </div>
    </div>
  </div>

<script>
const apiBase = location.origin;
let catalog = null;
let currentView = "catalog";
let selectedBlockId = "";
let catalogVer = 0;

function qs(sel) { return document.querySelector(sel); }
function qsa(sel) { return Array.from(document.querySelectorAll(sel)); }

function toast(kind, title, msg) {
  const t = qs("#toast");
  t.className = "toast " + (kind || "");
  qs("#toastTitle").textContent = title || "";
  qs("#toastMsg").textContent = msg || "";
  t.style.display = "block";
  setTimeout(() => { t.style.display = "none"; }, 3200);
}

function setStatus(kind, text) {
  qs("#statusText").textContent = text;
  const dot = qs("#statusDot");
  dot.style.background = kind === "ok" ? "var(--good)" : kind === "warn" ? "var(--warn)" : kind === "err" ? "var(--bad)" : "var(--muted2)";
  dot.style.boxShadow = kind === "ok" ? "0 0 0 6px rgba(34,197,94,.12)" :
                      kind === "warn" ? "0 0 0 6px rgba(245,158,11,.14)" :
                      kind === "err" ? "0 0 0 6px rgba(239,68,68,.14)" :
                      "0 0 0 6px rgba(122,122,133,.12)";
}

function switchView(name) {
  currentView = name;
  qs("#view-catalog").style.display = name === "catalog" ? "" : "none";
  qs("#view-blocks").style.display = name === "blocks" ? "" : "none";
  qs("#view-templates").style.display = name === "templates" ? "" : "none";
  qs("#nav-catalog").classList.toggle("active", name === "catalog");
  qs("#nav-blocks").classList.toggle("active", name === "blocks");
  qs("#nav-templates").classList.toggle("active", name === "templates");
}

function randID(prefix) { return prefix + "_" + Math.random().toString(16).slice(2, 10); }

async function loadCatalog() {
  const res = await fetch(apiBase + "/api/v1/catalog");
  if (!res.ok) throw new Error(await res.text());
  catalog = await res.json();
  catalog.categories = catalog.categories || [];
  catalog.blocks = catalog.blocks || [];
  catalog.mainTemplates = catalog.mainTemplates || [];
  qs("#nav-catalog-count").textContent = String(catalog.categories.length);
  qs("#nav-blocks-count").textContent = String(catalog.blocks.length);
  qs("#nav-templates-count").textContent = String(catalog.mainTemplates.length);
  renderCatalog();
  renderBlocks();
  renderTemplates();
}

function ensureCategoryOptions(selectEl, includeAll) {
  selectEl.innerHTML = "";
  if (includeAll) selectEl.appendChild(new Option("All categories", ""));
  for (const c of catalog.categories) {
    const label = (c.name || c.id || "Category") + " (" + (c.id || "") + ")";
    selectEl.appendChild(new Option(label, c.id));
  }
}

function renderCatalog() {
  const tbody = qs("#cat-table");
  tbody.innerHTML = "";
  const blocksByCat = new Map();
  for (const b of catalog.blocks) {
    const k = b.categoryId || "";
    blocksByCat.set(k, (blocksByCat.get(k) || 0) + 1);
  }
  for (const c of catalog.categories) {
    const tr = document.createElement("tr");
    const tdName = document.createElement("td");
    const tdId = document.createElement("td");
    const tdSel = document.createElement("td");
    const tdCount = document.createElement("td");
    const tdAct = document.createElement("td");

    const name = document.createElement("input");
    name.value = c.name || "";
    name.addEventListener("input", () => c.name = name.value);
    tdName.appendChild(name);

    const id = document.createElement("input");
    id.value = c.id || "";
    id.className = "mono";
    id.disabled = true;
    tdId.appendChild(id);

    const sel = document.createElement("select");
    sel.appendChild(new Option("multi", "multi"));
    sel.appendChild(new Option("single", "single"));
    sel.value = c.selectionMode || "multi";
    sel.addEventListener("change", () => c.selectionMode = sel.value);
    tdSel.appendChild(sel);

    tdCount.textContent = String(blocksByCat.get(c.id) || 0);
    tdCount.style.color = "var(--muted)";

    const del = document.createElement("button");
    del.className = "btn danger";
    del.textContent = "Delete";
    del.addEventListener("click", () => {
      catalog.categories = catalog.categories.filter(x => x.id !== c.id);
      renderCatalog();
      renderBlocks();
      renderTemplates();
    });
    tdAct.appendChild(del);

    tr.appendChild(tdName);
    tr.appendChild(tdId);
    tr.appendChild(tdSel);
    tr.appendChild(tdCount);
    tr.appendChild(tdAct);
    tbody.appendChild(tr);
  }
}

function depRow(name, ver, onChange, onDelete) {
  const wrap = document.createElement("div");
  wrap.className = "item";
  wrap.style.cursor = "default";
  const left = document.createElement("div");
  left.style.flex = "1";
  left.style.display = "grid";
  left.style.gridTemplateColumns = "1fr 1fr";
  left.style.gap = "10px";
  const inName = document.createElement("input");
  inName.className = "mono";
  inName.placeholder = "package";
  inName.value = name || "";
  const inVer = document.createElement("input");
  inVer.className = "mono";
  inVer.placeholder = "^1.0.0";
  inVer.value = ver || "";
  inName.addEventListener("input", () => onChange(inName.value, inVer.value));
  inVer.addEventListener("input", () => onChange(inName.value, inVer.value));
  left.appendChild(inName);
  left.appendChild(inVer);

  const del = document.createElement("button");
  del.className = "btn danger";
  del.textContent = "Remove";
  del.addEventListener("click", onDelete);
  wrap.appendChild(left);
  wrap.appendChild(del);
  return wrap;
}

function readBlockForm() {
  const b = {};
  b.id = qs("#b-id").value.trim();
  b.label = qs("#b-label").value.trim();
  b.categoryId = qs("#b-category").value;
  b.description = qs("#b-desc").value.trim();
  b.basePath = qs("#b-base").value.trim();
  b.mainTarget = qs("#b-main-target").value.trim();
  b.mainMode = qs("#b-main-mode").value;
  b.mainContent = qs("#b-main-content").value;
  b.conflicts = [];
  for (const cb of qsa("input[data-conflict]")) {
    if (cb.checked) b.conflicts.push(cb.getAttribute("data-conflict"));
  }
  b.deps = {};
  for (const row of qsa("[data-dep-row]")) {
    const n = row.querySelector("input[name='dep_name']").value.trim();
    const v = row.querySelector("input[name='dep_ver']").value.trim();
    if (n) b.deps[n] = v || "";
  }
  b.blobId = qs("#b-id").getAttribute("data-blob") || "";
  b.updatedAt = qs("#b-id").getAttribute("data-updated") || "";
  return b;
}

function writeBlockForm(b) {
  qs("#b-id").value = b?.id || "";
  qs("#b-id").setAttribute("data-blob", b?.blobId || "");
  qs("#b-id").setAttribute("data-updated", b?.updatedAt || "");
  qs("#b-label").value = b?.label || "";
  qs("#b-category").value = b?.categoryId || (catalog.categories[0]?.id || "");
  qs("#b-desc").value = b?.description || "";
  qs("#b-base").value = b?.basePath || "lib/";
  qs("#b-main-target").value = b?.mainTarget || "lib/main.dart";
  qs("#b-main-mode").value = b?.mainMode || "append";
  qs("#b-main-content").value = b?.mainContent || "";
  qs("#b-zip").value = "";
  qs("#btn-delete-block").disabled = !(b && b.id);

  const deps = qs("#deps");
  deps.innerHTML = "";
  const depsObj = b?.deps || {};
  const keys = Object.keys(depsObj);
  if (keys.length === 0) {
    addDepRow("", "");
  } else {
    for (const k of keys) addDepRow(k, depsObj[k]);
  }

  renderConflicts(b?.id || "", b?.conflicts || []);
  qs("#block-editor-sub").textContent = b && b.id ? "Editing " + b.id : "Create a new block";
}

function addDepRow(n, v) {
  const deps = qs("#deps");
  const row = document.createElement("div");
  row.className = "item";
  row.style.cursor = "default";
  row.setAttribute("data-dep-row", "1");

  const left = document.createElement("div");
  left.style.flex = "1";
  left.style.display = "grid";
  left.style.gridTemplateColumns = "1fr 1fr";
  left.style.gap = "10px";

  const inName = document.createElement("input");
  inName.name = "dep_name";
  inName.className = "mono";
  inName.placeholder = "package";
  inName.value = n || "";

  const inVer = document.createElement("input");
  inVer.name = "dep_ver";
  inVer.className = "mono";
  inVer.placeholder = "^1.0.0";
  inVer.value = v || "";

  left.appendChild(inName);
  left.appendChild(inVer);

  const del = document.createElement("button");
  del.className = "btn danger";
  del.textContent = "Remove";
  del.addEventListener("click", () => row.remove());

  row.appendChild(left);
  row.appendChild(del);
  deps.appendChild(row);
}

function renderConflicts(selfId, selected) {
  const root = qs("#conflicts");
  root.innerHTML = "";
  const set = new Set(selected || []);
  for (const b of catalog.blocks) {
    if (!b.id || b.id === selfId) continue;
    const row = document.createElement("div");
    row.className = "item";
    row.style.cursor = "default";
    const left = document.createElement("div");
    const t = document.createElement("div");
    t.className = "title";
    t.textContent = b.label || b.id;
    const m = document.createElement("div");
    m.className = "meta";
    m.textContent = b.id;
    left.appendChild(t);
    left.appendChild(m);
    const cb = document.createElement("input");
    cb.type = "checkbox";
    cb.setAttribute("data-conflict", b.id);
    cb.checked = set.has(b.id);
    row.appendChild(left);
    row.appendChild(cb);
    root.appendChild(row);
  }
}

function renderBlocks() {
  qs("#block-count").textContent = String(catalog.blocks.length) + " blocks";
  ensureCategoryOptions(qs("#b-category"), false);
  ensureCategoryOptions(qs("#block-category-filter"), true);

  const query = qs("#block-search").value.trim().toLowerCase();
  const catFilter = qs("#block-category-filter").value;

  const list = qs("#block-list");
  list.innerHTML = "";

  const blocks = (catalog.blocks || [])
    .slice()
    .sort((a,b) => String(a.label||a.id||"").localeCompare(String(b.label||b.id||"")));

  for (const b of blocks) {
    if (catFilter && (b.categoryId || "") !== catFilter) continue;
    const hay = (b.label || "") + " " + (b.id || "");
    if (query && !hay.toLowerCase().includes(query)) continue;
    const item = document.createElement("div");
    item.className = "item" + (b.id === selectedBlockId ? " active" : "");
    const left = document.createElement("div");
    const title = document.createElement("div");
    title.className = "title";
    title.textContent = b.label || b.id || "Untitled";
    const meta = document.createElement("div");
    meta.className = "meta";
    meta.textContent = b.id || "";
    left.appendChild(title);
    left.appendChild(meta);
    const right = document.createElement("div");
    right.className = "subtle";
    right.textContent = (b.categoryId || "—");
    item.appendChild(left);
    item.appendChild(right);
    item.addEventListener("click", () => {
      selectedBlockId = b.id;
      writeBlockForm(b);
      renderBlocks();
    });
    list.appendChild(item);
  }

  const selected = (catalog.blocks || []).find(x => x.id === selectedBlockId);
  if (selected) {
    renderConflicts(selected.id, selected.conflicts || []);
  } else {
    renderConflicts("", []);
  }
}

function renderTemplates() {
  const grid = qs("#tpl-grid");
  grid.innerHTML = "";
  for (const t of catalog.mainTemplates) {
    const card = document.createElement("div");
    card.className = "card";
    const header = document.createElement("div");
    header.className = "cardHeader";
    const left = document.createElement("div");
    const h = document.createElement("h3");
    h.textContent = t.label || "Template";
    const id = document.createElement("div");
    id.className = "subtle";
    id.style.fontFamily = "var(--code)";
    id.textContent = t.id;
    left.appendChild(h);
    left.appendChild(id);
    const del = document.createElement("button");
    del.className = "btn danger";
    del.textContent = "Delete";
    del.addEventListener("click", () => {
      catalog.mainTemplates = catalog.mainTemplates.filter(x => x.id !== t.id);
      renderTemplates();
    });
    header.appendChild(left);
    header.appendChild(del);

    const wrap = document.createElement("div");
    wrap.className = "row row1";

    const lLabel = document.createElement("div");
    const lab = document.createElement("label");
    lab.textContent = "Label";
    const inLabel = document.createElement("input");
    inLabel.value = t.label || "";
    inLabel.addEventListener("input", () => { t.label = inLabel.value; h.textContent = t.label || "Template"; });
    lLabel.appendChild(lab);
    lLabel.appendChild(inLabel);

    const lContent = document.createElement("div");
    const lc = document.createElement("label");
    lc.textContent = "Content (Dart). Placeholder: {{APP_TITLE}}";
    const ta = document.createElement("textarea");
    ta.className = "code";
    ta.spellcheck = false;
    ta.value = t.content || "";
    ta.addEventListener("input", () => t.content = ta.value);
    lContent.appendChild(lc);
    lContent.appendChild(ta);

    wrap.appendChild(lLabel);
    wrap.appendChild(lContent);

    card.appendChild(header);
    card.appendChild(wrap);
    grid.appendChild(card);
  }
}

async function saveCatalog() {
  qs("#btn-save-catalog").disabled = true;
  try {
    const payload = { categories: catalog.categories, mainTemplates: catalog.mainTemplates };
    const res = await fetch(apiBase + "/api/v1/admin/catalog", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!res.ok) throw new Error(await res.text());
    toast("ok", "Saved", "Catalog + templates saved.");
    await loadCatalog();
  } catch (e) {
    toast("err", "Save failed", e?.message || String(e));
  } finally {
    qs("#btn-save-catalog").disabled = false;
  }
}

async function saveBlock() {
  const b = readBlockForm();
  if (!b.label) return toast("err", "Missing label", "Please add a label.");
  if (!b.categoryId) return toast("err", "Missing category", "Please select a category.");
  if (!b.basePath) b.basePath = "lib/";
  if (!b.mainTarget) b.mainTarget = "lib/main.dart";

  const zip = qs("#b-zip").files && qs("#b-zip").files[0] ? qs("#b-zip").files[0] : null;
  const isNew = !b.id;

  try {
    qs("#btn-save-block").disabled = true;
    if (isNew || zip) {
      if (!zip) throw new Error("Zip file is required for new blocks.");
      const fd = new FormData();
      fd.append("meta", JSON.stringify(b));
      fd.append("file", zip);
      const res = await fetch(apiBase + "/api/v1/admin/blocks", { method: "POST", body: fd });
      if (!res.ok) throw new Error(await res.text());
      toast("ok", "Block saved", "Uploaded zip + metadata.");
    } else {
      const id = b.id;
      const res = await fetch(apiBase + "/api/v1/admin/blocks/" + encodeURIComponent(id), {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(b),
      });
      if (!res.ok) throw new Error(await res.text());
      toast("ok", "Block saved", "Metadata updated.");
    }
    await loadCatalog();
    selectedBlockId = b.id || selectedBlockId;
    renderBlocks();
  } catch (e) {
    toast("err", "Save failed", e?.message || String(e));
  } finally {
    qs("#btn-save-block").disabled = false;
  }
}

async function deleteBlock() {
  const id = (qs("#b-id").value || "").trim();
  if (!id) return;
  try {
    qs("#btn-delete-block").disabled = true;
    const res = await fetch(apiBase + "/api/v1/admin/blocks/" + encodeURIComponent(id), { method: "DELETE" });
    if (!res.ok) throw new Error(await res.text());
    toast("ok", "Deleted", "Block removed.");
    selectedBlockId = "";
    writeBlockForm({});
    await loadCatalog();
  } catch (e) {
    toast("err", "Delete failed", e?.message || String(e));
  } finally {
    qs("#btn-delete-block").disabled = false;
  }
}

function openModal(title, bodyNode, primaryText, onPrimary) {
  qs("#modalTitle").textContent = title;
  const body = qs("#modalBody");
  body.innerHTML = "";
  body.appendChild(bodyNode);
  qs("#modalPrimary").textContent = primaryText || "Save";
  qs("#modalBack").style.display = "flex";
  const primary = qs("#modalPrimary");
  const handler = async () => { await onPrimary(); closeModal(); };
  primary.onclick = handler;
}
function closeModal() { qs("#modalBack").style.display = "none"; }

async function connectStream() {
  try {
    const url = apiBase + "/api/v1/catalog/stream";
    const es = new EventSource(url);
    qs("#chip-stream").textContent = "stream: on";
    es.addEventListener("open", () => setStatus("ok", "Live sync connected"));
    es.addEventListener("error", () => setStatus("warn", "Live sync reconnecting…"));
    es.addEventListener("catalog", async (ev) => {
      const v = Number(ev.data || "0");
      if (!Number.isNaN(v)) {
        catalogVer = v;
        qs("#catalogVer").textContent = String(catalogVer);
      }
      await loadCatalog();
    });
  } catch (_) {
    setStatus("warn", "Live sync unavailable");
    qs("#chip-stream").textContent = "stream: off";
  }
}

qs("#nav-catalog").addEventListener("click", () => switchView("catalog"));
qs("#nav-blocks").addEventListener("click", () => switchView("blocks"));
qs("#nav-templates").addEventListener("click", () => switchView("templates"));
qs("#btn-open-admin").addEventListener("click", () => window.open(location.href, "_blank"));
qs("#btn-reload").addEventListener("click", async () => { await loadCatalog(); toast("ok", "Reloaded", "Catalog reloaded."); });
qs("#btn-save-catalog").addEventListener("click", saveCatalog);

qs("#btn-add-category").addEventListener("click", () => {
  const wrap = document.createElement("div");
  wrap.className = "row row1";

  const id = randID("cat");
  const n1 = document.createElement("div");
  const l1 = document.createElement("label");
  l1.textContent = "Name";
  const inName = document.createElement("input");
  inName.value = "New Category";
  n1.appendChild(l1);
  n1.appendChild(inName);

  const n2 = document.createElement("div");
  const l2 = document.createElement("label");
  l2.textContent = "Selection Mode";
  const sel = document.createElement("select");
  sel.appendChild(new Option("multi", "multi"));
  sel.appendChild(new Option("single", "single"));
  n2.appendChild(l2);
  n2.appendChild(sel);

  const n3 = document.createElement("div");
  const l3 = document.createElement("label");
  l3.textContent = "ID (auto)";
  const inId = document.createElement("input");
  inId.value = id;
  inId.disabled = true;
  inId.className = "mono";
  n3.appendChild(l3);
  n3.appendChild(inId);

  wrap.appendChild(n1);
  wrap.appendChild(n2);
  wrap.appendChild(n3);

  openModal("Add Category", wrap, "Add", async () => {
    catalog.categories.push({ id, name: inName.value, selectionMode: sel.value });
    renderCatalog();
    renderBlocks();
  });
});

qs("#btn-new-block").addEventListener("click", () => {
  selectedBlockId = "";
  writeBlockForm({});
  renderBlocks();
});
qs("#btn-add-dep").addEventListener("click", () => addDepRow("", ""));
qs("#btn-save-block").addEventListener("click", saveBlock);
qs("#btn-delete-block").addEventListener("click", deleteBlock);
qs("#block-search").addEventListener("input", renderBlocks);
qs("#block-category-filter").addEventListener("change", renderBlocks);

qs("#btn-add-template").addEventListener("click", () => {
  const id = randID("tpl");
  catalog.mainTemplates.push({ id, label: "New Template", content: "" });
  renderTemplates();
});

qs("#modalClose").addEventListener("click", closeModal);
qs("#modalBack").addEventListener("click", (e) => { if (e.target === qs("#modalBack")) closeModal(); });

(async function boot() {
  qs("#chip-api").textContent = "api: " + apiBase;
  setStatus("warn", "Loading…");
  try {
    await loadCatalog();
    setStatus("ok", "Ready");
    const selId = catalog.categories[0]?.id || "";
    qs("#b-category").value = selId;
    ensureCategoryOptions(qs("#block-category-filter"), true);
    writeBlockForm({});
    connectStream();
  } catch (e) {
    setStatus("err", "Failed to load");
    toast("err", "Load failed", e?.message || String(e));
  }
})();
</script>
</body>
</html>`
