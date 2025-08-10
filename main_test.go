package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var (
	origWD  string
	tmpRoot string
)

// TestMain runs before tests; set an isolated working directory so the
// app writes its ./data storage there. main.go's init() will already
// have created a ./data in the original WD; we remove it after tests.
func TestMain(m *testing.M) {
	// remember original working dir and create a temp root
	wd, _ := os.Getwd()
	origWD = wd
	tmp, err := os.MkdirTemp("", "blog-e2e-*")
	if err != nil {
		panic(err)
	}
	tmpRoot = tmp

	// switch to tmp root; ensure storage dir exists for tests
	if err := os.Chdir(tmpRoot); err != nil {
		panic(err)
	}
	_ = os.MkdirAll(storageDir, 0o755)

	code := m.Run()

	// cleanup: remove temp root and any ./data created in original WD by init()
	_ = os.Chdir(origWD)
	_ = os.RemoveAll(tmpRoot)
	_ = os.RemoveAll(filepath.Join(origWD, storageDir))
	os.Exit(code)
}

// buildMux mirrors main() so tests can exercise real routes.
func buildMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", homeHandler)
	mux.HandleFunc("/article/", articleHandler)
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
	return logRequest(mux)
}

// resetStorage ensures a clean ./data for each test.
func resetStorage(t *testing.T) {
	t.Helper()
	_ = os.RemoveAll(storageDir)
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", storageDir, err)
	}
}

func TestMakeSlug(t *testing.T) {
	if got := makeSlug(" Hello,  World!  "); !strings.HasPrefix(got, "hello--world") && got != "hello-world" {
		t.Fatalf("unexpected slug: %q", got)
	}
	if got := makeSlug(" "); !strings.HasPrefix(got, "post-") {
		t.Fatalf("empty title should fallback to post-*: %q", got)
	}
}

func TestHome_Empty(t *testing.T) {
	resetStorage(t)
	ts := httptest.NewServer(buildMux())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(b))
	}
	if !strings.Contains(string(b), "No articles yet.") {
		t.Fatalf("home should show empty state, got:\n%s", string(b))
	}
}

// login returns the Set-Cookie header value for authenticated session.
func login(t *testing.T, base string, user, pass string) string {
	t.Helper()
	form := url.Values{"username": {user}, "password": {pass}}
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.PostForm(base+"/admin/login", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login status=%d, want 302", resp.StatusCode)
	}
	ck := resp.Header.Get("Set-Cookie")
	if ck == "" {
		t.Fatalf("missing Set-Cookie on login")
	}
	return ck
}

func TestAuth_Protection(t *testing.T) {
	resetStorage(t)
	ts := httptest.NewServer(buildMux())
	defer ts.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(ts.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("/admin status=%d, want 302", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/admin/login" {
		t.Fatalf("redirect to %q, want /admin/login", loc)
	}
}

func TestAdmin_Login_And_Dashboard(t *testing.T) {
	resetStorage(t)
	ts := httptest.NewServer(buildMux())
	defer ts.Close()

	cookie := login(t, ts.URL, adminUser, adminPass)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/admin", nil)
	req.Header.Set("Cookie", cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard status=%d", resp.StatusCode)
	}
	if !strings.Contains(string(b), "Dashboard") {
		t.Fatalf("dashboard content missing: %s", string(b))
	}
}

func TestArticle_Create_View_Delete_Flow(t *testing.T) {
	resetStorage(t)
	ts := httptest.NewServer(buildMux())
	defer ts.Close()

	cookie := login(t, ts.URL, adminUser, adminPass)

	// Create a new article via /admin/new
	form := url.Values{}
	form.Set("title", "Hello Test World")
	form.Set("content", "This is the content.\nWith new lines.")
	form.Set("date", "2024-01-02")
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/admin/new", strings.NewReader(form.Encode()))
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("create status=%d, want 302", resp.StatusCode)
	}

	slug := makeSlug("Hello Test World")

	// Home should list the new article
	resHome, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resHome.Body.Close()
	bodyHome, _ := io.ReadAll(resHome.Body)
	if !strings.Contains(string(bodyHome), "Hello Test World") {
		t.Fatalf("home missing article title: %s", string(bodyHome))
	}

	// Article page should show content and formatted date
	resArt, err := http.Get(ts.URL + "/article/" + slug)
	if err != nil {
		t.Fatal(err)
	}
	defer resArt.Body.Close()
	bodyArt, _ := io.ReadAll(resArt.Body)
	if resArt.StatusCode != http.StatusOK {
		t.Fatalf("article status=%d", resArt.StatusCode)
	}
	if !strings.Contains(string(bodyArt), "This is the content.") {
		t.Fatalf("article content missing: %s", string(bodyArt))
	}
	// date format used in templates is Jan 02, 2006
	d, _ := time.Parse("2006-01-02", "2024-01-02")
	expectedDate := d.Format("Jan 02, 2006")
	if !strings.Contains(string(bodyArt), expectedDate) {
		t.Fatalf("article date missing: want %q in body: %s", expectedDate, string(bodyArt))
	}

	// Delete the article
	delReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/admin/delete/"+slug, nil)
	delReq.Header.Set("Cookie", cookie)
	delResp, err := client.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusFound {
		t.Fatalf("delete status=%d, want 302", delResp.StatusCode)
	}

	// Article should now be 404
	check, _ := http.Get(ts.URL + "/article/" + slug)
	defer check.Body.Close()
	if check.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(check.Body)
		t.Fatalf("expected 404 after delete, got %d body=%s", check.StatusCode, string(b))
	}
}
