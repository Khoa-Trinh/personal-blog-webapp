# 📝 Personal Blog (Go)

A small, server‑rendered personal blog built with **Go**, **net/http**, and **html/template**. It has a public (guest) section for reading posts and a protected (admin) section to create, edit, and delete articles. Storage is the local filesystem — no database required.

> Project idea inspired by: https://roadmap.sh/projects/personal-blog

---

## ✨ Features

- **Guest**
    - **Home**: list all articles (newest first)
    - **Article**: view a single article with its publication date
- **Admin** (login required)
    - **Dashboard**: list all articles with quick actions
    - **Add Article**: title, content, date (YYYY-MM-DD)
    - **Edit Article**: update title/content/date; slug auto-updates when title changes
    - **Delete Article**: removes from filesystem
- **Storage**: articles saved as individual JSON files in `./data/`
- **Templating**: clean, modern styling using pure HTML/CSS and Go templates
- **No JS needed**: forms post back to the server, responses rendered on the server

---

## 🏗️ Architecture

- **Language**: Go
- **Server**: `net/http`
- **Templates**: `html/template` (partials compiled into `main.go`)
- **Storage**: JSON files on disk (one file per article)
- **Auth**: simple session cookie after a username/password form login

```
.
├── main.go          # server, routes, handlers, templates, storage ops
└── data/            # (created automatically) article JSON files live here
```

> If you also add a `main_test.go` (optional), you can run `go test -v` for integration tests.

---

## 🚀 Getting Started

### 1) Requirements
- Go 1.20+

### 2) Configure (optional)
Open `main.go` and change the admin credentials:
```go
const (
    adminUser = "admin"
    adminPass = "changeme" // ← change this
)
```

### 3) Run
```bash
go run main.go
```
Visit:
- **Guest Home**: http://localhost:8080/
- **Admin Dashboard**: http://localhost:8080/admin  
  Login with your configured credentials (default: `admin / changeme`).

> The server will automatically create the `./data/` directory if it’s missing.

---

## 🗂️ Storage Format
Each article is a JSON file in `./data/`:
```json
{
  "title": "My First Post",
  "slug": "my-first-post",
  "content": "Hello world! This is my first post.",
  "published": "2024-01-02T00:00:00Z"
}
```
- **Slug** is derived from the title. When editing a title, the slug (and filename) may change.
- **Published** is stored as an ISO‑8601 timestamp; form input is `YYYY-MM-DD`.

---

## 🌐 Routes & Pages

### Guest
- `GET /` – Home; lists all posts (newest first)
- `GET /article/{slug}` – Article page

### Admin
- `GET /admin/login` – Login form
- `POST /admin/login` – Create session on success
- `GET /admin/logout` – Clear session
- `GET /admin` – Dashboard (requires auth)
- `GET /admin/new` – New article form (requires auth)
- `POST /admin/new` – Persist new article (requires auth)
- `GET /admin/edit/{slug}` – Edit form (requires auth)
- `POST /admin/edit/{slug}` – Save edits (requires auth)
- `POST /admin/delete/{slug}` – Delete article (requires auth)

> Auth is a minimal session stored in memory (cookie named `session`). For production, replace with a robust auth layer and persistent sessions.

---

## 🎨 Templates & Styling
- All templates are defined in `main.go` and registered into a single `template.Template`.
- Minimal, responsive CSS is embedded; no external CSS/JS dependencies.

---

## 🧪 Testing (Optional)
If you add the provided `main_test.go` file, run:
```bash
go test -v
```
It includes:
- Slug generation tests
- Auth protection and login flow
- Create → View → Delete article end‑to‑end

---

## 🔒 Notes & Caveats
- The admin password is hardcoded for simplicity — **change it** before sharing the app.
- Sessions are stored in memory; restarting the server logs you out.
- No CSRF protection, roles, or password hashing are included (out of scope). Add these if you deploy publicly.

---

## 🛠️ Extending Ideas
- Markdown rendering for article content
- Categories / tags and filtering on Home
- Search by title/content
- Draft vs. published states
- Pagination for many posts
- RSS feed
- File uploads for cover images

---

## 📜 License
MIT — free to use, modify, and share. -- see [LICENSE](LICENSE) for details.
