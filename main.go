package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// --------------------------- Config ---------------------------
const (
	listenAddr    = ":8080"
	storageDir    = "data"
	adminUser     = "admin"
	adminPass     = "changeme" // change this
	sessionCookie = "session"
	sessionMaxAge = 24 * 3600 // 1 day
)

// --------------------------- Types ----------------------------

type Article struct {
	Title     string    `json:"title"`
	Slug      string    `json:"slug"`
	Content   string    `json:"content"`
	Published time.Time `json:"published"`
}

// --------------------------- Globals --------------------------

var (
	tmpl = template.Must(template.New("base").Funcs(template.FuncMap{
		"date": func(t time.Time) string { return t.Format("Jan 02, 2006") },
		"dateInput": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("2006-01-02")
		},
	}).Parse(baseHTML))

	sessions = map[string]bool{} // token -> isAdmin
)

func init() {
	// Parse subtemplates into the same set.
	template.Must(tmpl.New("home").Parse(homeHTML))
	template.Must(tmpl.New("article").Parse(articleHTML))
	template.Must(tmpl.New("admin_login").Parse(adminLoginHTML))
	template.Must(tmpl.New("admin_dashboard").Parse(adminDashboardHTML))
	template.Must(tmpl.New("admin_form").Parse(adminFormHTML))

	// Ensure storage dir exists on startup (handles missing ./data gracefully).
	if err := ensureStorage(); err != nil {
		log.Fatalf("storage init: %v", err)
	}
}

// --------------------------- Storage --------------------------

// ensureStorage creates the storage directory if it doesn't exist.
func ensureStorage() error {
	return os.MkdirAll(storageDir, 0o755)
}

func allArticles() ([]Article, error) {
	// If the data folder is missing, create it and return empty list.
	if err := ensureStorage(); err != nil { // handles missing ./data
		return nil, err
	}
	var list []Article
	err := filepath.WalkDir(storageDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var a Article
		if err := json.Unmarshal(b, &a); err != nil {
			return err
		}
		list = append(list, a)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// newest first
	sort.Slice(list, func(i, j int) bool { return list[i].Published.After(list[j].Published) })
	return list, nil
}

func loadArticle(slug string) (Article, error) {
	if err := ensureStorage(); err != nil {
		return Article{}, err
	}
	p := filepath.Join(storageDir, slug+".json")
	b, err := os.ReadFile(p)
	if err != nil {
		return Article{}, err
	}
	var a Article
	if err := json.Unmarshal(b, &a); err != nil {
		return Article{}, err
	}
	return a, nil
}

func saveArticle(a Article) error {
	if a.Slug == "" {
		return errors.New("missing slug")
	}
	if err := ensureStorage(); err != nil {
		return err
	}
	p := filepath.Join(storageDir, a.Slug+".json")
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func deleteArticle(slug string) error {
	if err := ensureStorage(); err != nil {
		return err
	}
	p := filepath.Join(storageDir, slug+".json")
	err := os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// --------------------------- Util -----------------------------

func makeSlug(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	// keep letters, digits, dash only
	var out strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			out.WriteRune(r)
		}
	}
	slug := out.String()
	if slug == "" {
		slug = fmt.Sprintf("post-%d", time.Now().Unix())
	}
	return slug
}

func newToken(n int) string {
	if n <= 0 {
		n = 32
	}
	b := make([]byte, n)
	_, _ = rand.Read(b)
	const alpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for i := range b {
		b[i] = alpha[int(b[i])%len(alpha)]
	}
	return string(b)
}

func isAuthed(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return sessions[c.Value]
}

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAuthed(r) {
			http.Redirect(w, r, "/admin/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

// --------------------------- Handlers (Guest) -----------------

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	arts, err := allArticles()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	data := map[string]any{
		"Active":   "home",
		"Articles": arts,
		"Title":    "Home",
	}
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func articleHandler(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/article/")
	if slug == "" {
		http.NotFound(w, r)
		return
	}
	a, err := loadArticle(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data := map[string]any{
		"Active":  "article",
		"Title":   a.Title,
		"Article": a,
	}
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

// --------------------------- Handlers (Admin) -----------------

func adminLoginGet(w http.ResponseWriter, r *http.Request, errMsg string) {
	data := map[string]any{"Active": "admin_login", "Title": "Admin Login", "Error": errMsg}
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func adminLoginPost(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	u := strings.TrimSpace(r.FormValue("username"))
	p := strings.TrimSpace(r.FormValue("password"))
	if u == adminUser && p == adminPass {
		tok := newToken(32)
		sessions[tok] = true
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: tok, Path: "/", MaxAge: sessionMaxAge, HttpOnly: true})
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	adminLoginGet(w, r, "Invalid credentials")
}

func adminLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		delete(sessions, c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	http.Redirect(w, r, "/admin/login", http.StatusFound)
}

func adminDashboard(w http.ResponseWriter, r *http.Request) {
	arts, err := allArticles()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	data := map[string]any{"Active": "admin_dashboard", "Title": "Dashboard", "Articles": arts}
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func adminNewGet(w http.ResponseWriter, r *http.Request, a *Article, errMsg string) {
	data := map[string]any{"Active": "admin_form", "Title": "Add Article", "Article": a, "Error": errMsg, "Mode": "add"}
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func adminNewPost(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	dateStr := strings.TrimSpace(r.FormValue("date"))
	if title == "" || content == "" || dateStr == "" {
		adminNewGet(w, r, nil, "All fields are required")
		return
	}
	pub, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		adminNewGet(w, r, nil, "Invalid date (use YYYY-MM-DD)")
		return
	}
	a := Article{Title: title, Slug: makeSlug(title), Content: content, Published: pub}
	if err := saveArticle(a); err != nil {
		adminNewGet(w, r, &a, err.Error())
		return
	}
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func adminEditGet(w http.ResponseWriter, r *http.Request, a *Article, errMsg string) {
	slug := strings.TrimPrefix(r.URL.Path, "/admin/edit/")
	var art Article
	var err error
	if a != nil {
		art = *a
	} else {
		art, err = loadArticle(slug)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	data := map[string]any{"Active": "admin_form", "Title": "Edit Article", "Article": &art, "Error": errMsg, "Mode": "edit"}
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func adminEditPost(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	slug := strings.TrimPrefix(r.URL.Path, "/admin/edit/")
	orig, err := loadArticle(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	dateStr := strings.TrimSpace(r.FormValue("date"))
	if title == "" || content == "" || dateStr == "" {
		adminEditGet(w, r, &orig, "All fields are required")
		return
	}
	pub, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		adminEditGet(w, r, &orig, "Invalid date (use YYYY-MM-DD)")
		return
	}

	newSlug := makeSlug(title)
	updated := Article{Title: title, Slug: newSlug, Content: content, Published: pub}
	if newSlug != orig.Slug {
		// rename file: save new then delete old
		if err := saveArticle(updated); err != nil {
			adminEditGet(w, r, &updated, err.Error())
			return
		}
		_ = deleteArticle(orig.Slug)
	} else {
		if err := saveArticle(updated); err != nil {
			adminEditGet(w, r, &updated, err.Error())
			return
		}
	}
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func adminDeletePost(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/admin/delete/")
	if slug == "" {
		http.NotFound(w, r)
		return
	}
	if err := deleteArticle(slug); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusFound)
}

// --------------------------- main -----------------------------

func main() {
	mux := http.NewServeMux()

	// guest
	mux.HandleFunc("/", homeHandler)
	mux.HandleFunc("/article/", articleHandler)

	// admin auth
	mux.HandleFunc("/admin/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			adminLoginGet(w, r, "")
			return
		}
		if r.Method == http.MethodPost {
			adminLoginPost(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/admin/logout", adminLogout)

	// admin protected
	mux.HandleFunc("/admin", requireAuth(adminDashboard))
	mux.HandleFunc("/admin/new", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			adminNewGet(w, r, nil, "")
			return
		}
		if r.Method == http.MethodPost {
			adminNewPost(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}))
	mux.HandleFunc("/admin/edit/", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			adminEditGet(w, r, nil, "")
			return
		}
		if r.Method == http.MethodPost {
			adminEditPost(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}))
	mux.HandleFunc("/admin/delete/", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			adminDeletePost(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}))

	log.Printf("Personal Blog running on http://localhost%s\n", listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, logRequest(mux)))
}

// basic request logger
func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// --------------------------- Templates ------------------------

const baseHTML = `{{define "base"}}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1"/>
  <title>{{.Title}} · Personal Blog</title>
  <style>
    :root{--bg:#0b0c10;--card:#15171c;--text:#e8eef2;--muted:#aab4bf;--accent:#60a5fa;--bad:#ef4444}
    *{box-sizing:border-box}
    body{margin:0;font-family:system-ui,-apple-system,Segoe UI,Roboto,Inter,Arial,sans-serif;background:var(--bg);color:var(--text)}
    header{padding:18px 20px;border-bottom:1px solid #23262d;background:#0f1116;position:sticky;top:0}
    a{color:var(--accent);text-decoration:none}
    nav a{color:var(--muted);margin-right:14px;padding:8px 10px;border-radius:10px}
    nav a.active{background:#15171c;color:#fff;border:1px solid #262a33}
    main{max-width:860px;margin:28px auto;padding:0 16px}
    .card{background:#15171c;border:1px solid #23262d;border-radius:16px;padding:20px;margin-bottom:16px}
    .row{display:grid;grid-template-columns:1fr 1fr;gap:12px}
    input, textarea{width:100%;background:#0f1116;border:1px solid #23262d;color:#e8eef2;padding:12px;border-radius:12px}
    textarea{min-height:260px}
    label{font-size:14px;color:var(--muted)}
    button{background:var(--accent);border:none;color:#04121f;padding:10px 14px;border-radius:12px;font-weight:600;cursor:pointer}
    table{width:100%;border-collapse:collapse}
    th,td{padding:10px;border-bottom:1px solid #23262d}
    .danger{background:#2a1111;border:1px solid #3a1a1a;color:#ffb4b4}
    .muted{color:var(--muted)}
  </style>
</head>
<body>
  <header>
    <nav>
      <a href="/" class="{{if eq .Active "home"}}active{{end}}">Home</a>
      <a href="/admin" class="{{if eq .Active "admin_dashboard"}}active{{end}}">Admin</a>
      {{if eq .Active "admin_login"}}<span class="muted">Login</span>{{end}}
    </nav>
  </header>
  <main>
    {{/* Render page partials */}}
    {{if eq .Active "home"}}
      {{template "home" .}}
    {{else if eq .Active "article"}}
      {{template "article" .}}
    {{else if eq .Active "admin_login"}}
      {{template "admin_login" .}}
    {{else if eq .Active "admin_dashboard"}}
      {{template "admin_dashboard" .}}
    {{else if eq .Active "admin_form"}}
      {{template "admin_form" .}}
    {{end}}
  </main>
</body>
</html>
{{end}}`

const homeHTML = `{{define "home"}}
  <h1 style="margin:0 0 12px 0">Personal Blog</h1>
  {{if not .Articles}}
    <div class="card">No articles yet.</div>
  {{end}}
  {{range .Articles}}
    <article class="card">
      <h2 style="margin:0 0 8px 0"><a href="/article/{{.Slug}}">{{.Title}}</a></h2>
      <div class="muted">Published {{date .Published}}</div>
    </article>
  {{end}}
{{end}}`

const articleHTML = `{{define "article"}}
  <article class="card">
    <h1 style="margin:0 0 8px 0">{{.Article.Title}}</h1>
    <div class="muted" style="margin-bottom:16px">Published {{date .Article.Published}}</div>
    <div style="white-space:pre-wrap;line-height:1.6">{{.Article.Content}}</div>
  </article>
{{end}}`

const adminLoginHTML = `{{define "admin_login"}}
  <div class="card">
    <h2>Admin Login</h2>
    {{if .Error}}<div class="card danger" style="margin-top:8px">{{.Error}}</div>{{end}}
    <form method="post" action="/admin/login" target="_self" autocomplete="off">
      <div class="row">
        <div>
          <label>Username</label>
          <input name="username" placeholder="admin" />
        </div>
        <div>
          <label>Password</label>
          <input name="password" type="password" placeholder="changeme" />
        </div>
      </div>
      <div style="margin-top:12px"><button type="submit">Sign in</button></div>
    </form>
    <div class="muted" style="margin-top:10px">Default credentials: admin / changeme</div>
  </div>
{{end}}`

const adminDashboardHTML = `{{define "admin_dashboard"}}
  <div class="card" style="display:flex;justify-content:space-between;align-items:center">
    <div><h2 style="margin:0">Dashboard</h2></div>
    <div>
      <a href="/admin/new"><button>Add Article</button></a>
      <a href="/admin/logout" style="margin-left:8px"><button class="danger">Logout</button></a>
    </div>
  </div>
  <div class="card">
    <table>
      <thead>
        <tr><th>Title</th><th>Published</th><th style="width:220px">Actions</th></tr>
      </thead>
      <tbody>
        {{if not .Articles}}
          <tr><td colspan="3" class="muted">No articles yet.</td></tr>
        {{end}}
        {{range .Articles}}
        <tr>
          <td><a href="/article/{{.Slug}}">{{.Title}}</a></td>
          <td>{{date .Published}}</td>
          <td>
            <a href="/admin/edit/{{.Slug}}"><button>Edit</button></a>
            <form method="post" action="/admin/delete/{{.Slug}}" style="display:inline">
              <button type="submit" class="danger">Delete</button>
            </form>
          </td>
        </tr>
        {{end}}
      </tbody>
    </table>
  </div>
{{end}}`

const adminFormHTML = `{{define "admin_form"}}
  <div class="card">
    <h2>{{if eq .Mode "add"}}Add Article{{else}}Edit Article{{end}}</h2>
    {{if .Error}}<div class="card danger" style="margin-top:8px">{{.Error}}</div>{{end}}
    <form method="post" action="{{if eq .Mode "add"}}/admin/new{{else}}/admin/edit/{{.Article.Slug}}{{end}}" target="_self" autocomplete="off">
      <div class="row">
        <div>
          <label>Title</label>
          <input name="title" value="{{if .Article}}{{.Article.Title}}{{end}}" />
        </div>
        <div>
          <label>Date of publication</label>
          <input name="date" type="date" value="{{if .Article}}{{dateInput .Article.Published}}{{end}}" placeholder="YYYY-MM-DD" />
        </div>
      </div>
      <div style="margin-top:12px">
        <label>Content</label>
        <textarea name="content" placeholder="Write your article...">{{if .Article}}{{.Article.Content}}{{end}}</textarea>
      </div>
      <div style="margin-top:12px">
        <button type="submit">{{if eq .Mode "add"}}Publish{{else}}Save Changes{{end}}</button>
        <a href="/admin" style="margin-left:8px">Cancel</a>
      </div>
    </form>
    {{if and .Article (ne .Mode "add")}}
      <div class="muted" style="margin-top:8px">Slug: {{.Article.Slug}}</div>
    {{end}}
  </div>
{{end}}`
