// Command gitViewer serves a local Git repository as a minimal web UI.
//
// It is similar in spirit to `python -m http.server`, but oriented around
// exploring a Git repository rather than a plain directory.
//
// Usage:
//
//	gitViewer [-addr :8080] [repo]
//
// If no repo path is provided, the current working directory is used.
package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/app.css
var appCSSContent string

//go:embed static/app.js
var appJSContent string

// Server serves a Git repository over HTTP.
type Server struct {
	repoPath string
	repoName string
	tmpls    map[string]*template.Template
}

// BaseData contains fields shared by all page templates.
type BaseData struct {
	RepoName      string
	Ref           string
	Branches      []string
	HasGHPages    bool     // Kept for backward compatibility
	PagesBranches []string // All branches available for pages viewing
}

// IndexData contains data for the overview page.
type IndexData struct {
	BaseData
	HeadHash string
}

// TreeEntry represents a single entry in a Git tree.
type TreeEntry struct {
	Name string
	Mode string
	Type string // "blob" or "tree"
	Size int64
}

// TreeData contains data for the tree browser page.
type TreeData struct {
	BaseData
	Path       string
	ParentPath string
	Entries    []TreeEntry
}

// BlobData contains data for the file viewer page.
type BlobData struct {
	BaseData
	Path      string
	Content   string
	Truncated bool
}

// CommitsData contains data for the commit list page.
type CommitsData struct {
	BaseData
	Commits []Commit
}

// Commit represents a single Git commit in the log listing.
type Commit struct {
	Hash    string
	Date    string
	Subject string
}

// DiffData contains data for the diff view page.
type DiffData struct {
	BaseData
	From  string
	To    string
	Patch string
}

// WorkflowsData contains data for the GitHub Actions workflow list page.
type WorkflowsData struct {
	BaseData
	Ref       string
	Workflows []string
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	repoPath := "."
	if flag.NArg() > 0 {
		repoPath = flag.Arg(0)
	}

	srv, err := newServer(repoPath)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	log.Printf("Serving %q on http://%s", srv.repoPath, *addr)
	if err := http.ListenAndServe(*addr, loggingMiddleware(srv.routes())); err != nil {
		log.Fatal(err)
	}
}

// newServer constructs a Server for the given repository path.
func newServer(repoPath string) (*Server, error) {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	top, err := runGit(strings.TrimSpace(abs), "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not a git repo (rev-parse --show-toplevel failed): %w", err)
	}
	top = strings.TrimSpace(top)
	repoName := filepath.Base(top)

	// Parse layout first and then create a per-page template by cloning
	funcMap := template.FuncMap{"parentPath": parentPath}
	base := template.Must(template.New("layout").Funcs(funcMap).ParseFS(templatesFS, "templates/layout.html"))

	// collect page templates
	tpls := make(map[string]*template.Template)
	entries, err := fs.ReadDir(templatesFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("read templates dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "layout.html" || !strings.HasSuffix(name, ".html") {
			continue
		}
		page := strings.TrimSuffix(name, ".html")

		// clone base so this page's block definitions won't affect others
		clone, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("clone base template: %w", err)
		}
		if _, err := clone.ParseFS(templatesFS, "templates/"+name); err != nil {
			return nil, fmt.Errorf("parse page %s: %w", name, err)
		}
		tpls[page] = clone
	}

	return &Server{
		repoPath: top,
		repoName: repoName,
		tmpls:    tpls,
	}, nil
}

// routes builds the HTTP handler tree for the server.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/tree", s.handleTree)
	mux.HandleFunc("/blob", s.handleBlob)
	mux.HandleFunc("/raw", s.handleRaw)
	mux.HandleFunc("/commits", s.handleCommits)
	mux.HandleFunc("/diff", s.handleDiff)
	mux.HandleFunc("/pages/", s.handlePages)
	mux.HandleFunc("/workflows", s.handleWorkflows)
	mux.HandleFunc("/static/app.css", handleAppCSS)
	mux.HandleFunc("/static/app.js", handleAppJS)
	return mux
}

// handleIndex renders the overview page for the repository.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	headRef, headHash, err := gitHead(s.repoPath)
	if err != nil {
		s.httpError(w, r, http.StatusInternalServerError, "Failed to read HEAD", err)
		return
	}

	base, err := s.baseData(headRef)
	if err != nil {
		s.httpError(w, r, http.StatusInternalServerError, "Failed to load repo metadata", err)
		return
	}

	data := IndexData{
		BaseData: base,
		HeadHash: headHash,
	}

	t, ok := s.tmpls["index"]
	if !ok {
		log.Printf("template not found: index")
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "index", data); err != nil {
		log.Printf("render index: %v", err)
	}
}

// handleTree renders a directory listing for a given ref and path.
func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	path := normalizeRepoPath(r.URL.Query().Get("path"))

	if ref == "" {
		headRef, _, err := gitHead(s.repoPath)
		if err != nil {
			s.httpError(w, r, http.StatusInternalServerError, "Failed to read HEAD", err)
			return
		}
		ref = headRef
	}

	base, err := s.baseData(ref)
	if err != nil {
		s.httpError(w, r, http.StatusInternalServerError, "Failed to load repo metadata", err)
		return
	}

	entries, err := gitLsTree(s.repoPath, ref, path)
	if err != nil {
		s.httpError(w, r, http.StatusInternalServerError, "Failed to read tree", err)
		return
	}

	parent := parentPath(path)

	data := TreeData{
		BaseData:   base,
		Path:       path,
		ParentPath: parent,
		Entries:    entries,
	}

	t, ok := s.tmpls["tree"]
	if !ok {
		log.Printf("template not found: tree")
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "tree", data); err != nil {
		log.Printf("render tree: %v", err)
	}
}

// handleBlob renders a file content page for a given ref and path.
func (s *Server) handleBlob(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	path := normalizeRepoPath(r.URL.Query().Get("path"))
	if ref == "" || path == "" {
		s.httpError(w, r, http.StatusBadRequest, "ref and path are required", nil)
		return
	}

	base, err := s.baseData(ref)
	if err != nil {
		s.httpError(w, r, http.StatusInternalServerError, "Failed to load repo metadata", err)
		return
	}

	spec := fmt.Sprintf("%s:%s", ref, path)
	content, err := gitShowFile(s.repoPath, spec)
	if err != nil {
		s.httpError(w, r, http.StatusInternalServerError, "Failed to read file", err)
		return
	}

	const maxPreview = 200 * 1024 // 200 KiB
	truncated := len(content) > maxPreview
	if truncated {
		content = content[:maxPreview]
	}

	data := BlobData{
		BaseData:  base,
		Path:      path,
		Content:   string(content),
		Truncated: truncated,
	}

	t, ok := s.tmpls["blob"]
	if !ok {
		log.Printf("template not found: blob")
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "blob", data); err != nil {
		log.Printf("render blob: %v", err)
	}
}

// handleRaw streams raw file bytes for a given ref and path.
func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	path := normalizeRepoPath(r.URL.Query().Get("path"))
	if ref == "" || path == "" {
		s.httpError(w, r, http.StatusBadRequest, "ref and path are required", nil)
		return
	}

	spec := fmt.Sprintf("%s:%s", ref, path)
	content, err := gitShowFile(s.repoPath, spec)
	if err != nil {
		s.httpError(w, r, http.StatusInternalServerError, "Failed to read file", err)
		return
	}

	ext := filepath.Ext(path)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(content)
}

// handleCommits renders a short commit log for the given ref.
func (s *Server) handleCommits(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		headRef, _, err := gitHead(s.repoPath)
		if err != nil {
			s.httpError(w, r, http.StatusInternalServerError, "Failed to read HEAD", err)
			return
		}
		ref = headRef
	}

	base, err := s.baseData(ref)
	if err != nil {
		s.httpError(w, r, http.StatusInternalServerError, "Failed to load repo metadata", err)
		return
	}

	commits, err := gitLog(s.repoPath, ref, 50)
	if err != nil {
		s.httpError(w, r, http.StatusInternalServerError, "Failed to read commits", err)
		return
	}

	data := CommitsData{
		BaseData: base,
		Commits:  commits,
	}

	t, ok := s.tmpls["commits"]
	if !ok {
		log.Printf("template not found: commits")
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "commits", data); err != nil {
		log.Printf("render commits: %v", err)
	}
}

// handleDiff renders a diff between two commits or refs.
func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		s.httpError(w, r, http.StatusBadRequest, "from and to query parameters are required", nil)
		return
	}

	// Use "to" as the current ref for nav.
	base, err := s.baseData(to)
	if err != nil {
		s.httpError(w, r, http.StatusInternalServerError, "Failed to load repo metadata", err)
		return
	}

	patch, err := gitDiff(s.repoPath, from, to)
	if err != nil {
		s.httpError(w, r, http.StatusInternalServerError, "Failed to compute diff", err)
		return
	}

	data := DiffData{
		BaseData: base,
		From:     from,
		To:       to,
		Patch:    patch,
	}

	t, ok := s.tmpls["diff"]
	if !ok {
		log.Printf("template not found: diff")
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "diff", data); err != nil {
		log.Printf("render diff: %v", err)
	}
}

// handlePages serves the contents of any branch as a static site.
//
// It maps /pages/{branch}/{path} to {branch}:{path}.
// Branch names can contain slashes. The function tries progressively longer
// prefixes as potential branch names until it finds a match.
// For backward compatibility, /pages/ without a branch defaults to gh-pages.
func (s *Server) handlePages(w http.ResponseWriter, r *http.Request) {
	// Extract path after /pages/
	pathAfterPages := strings.TrimPrefix(r.URL.Path, "/pages/")
	
	// Default to gh-pages for backward compatibility
	if pathAfterPages == "" {
		pathAfterPages = "gh-pages/"
	}
	
	// Split the path into segments
	segments := strings.Split(strings.TrimSuffix(pathAfterPages, "/"), "/")
	
	// Try progressively longer prefixes as potential branch names
	// Start from the longest possible branch name (most segments)
	var branch string
	var subPath string
	found := false
	
	for i := len(segments); i > 0; i-- {
		potentialBranch := strings.Join(segments[:i], "/")
		has, err := gitHasBranch(s.repoPath, potentialBranch)
		if err != nil {
			s.httpError(w, r, http.StatusInternalServerError, "Failed to check branch", err)
			return
		}
		if has {
			branch = potentialBranch
			if i < len(segments) {
				subPath = strings.Join(segments[i:], "/")
			}
			// Add trailing content if URL ended with slash
			if strings.HasSuffix(pathAfterPages, "/") && subPath != "" {
				subPath = subPath + "/"
			}
			found = true
			break
		}
	}
	
	// If no valid branch found, default to gh-pages
	if !found {
		branch = "gh-pages"
		subPath = pathAfterPages
		// Verify gh-pages exists
		has, err := gitHasBranch(s.repoPath, "gh-pages")
		if err != nil {
			s.httpError(w, r, http.StatusInternalServerError, "Failed to check gh-pages branch", err)
			return
		}
		if !has {
			s.httpError(w, r, http.StatusNotFound, "No valid branch found in path and gh-pages branch does not exist", nil)
			return
		}
	}

	subPath = normalizeRepoPath(subPath)

	if subPath == "" || strings.HasSuffix(subPath, "/") {
		subPath = subPath + "index.html"
	}

	spec := fmt.Sprintf("%s:%s", branch, subPath)
	content, err := gitShowFile(s.repoPath, spec)
	if err != nil {
		s.httpError(w, r, http.StatusNotFound, fmt.Sprintf("File not found in %s", branch), err)
		return
	}

	ext := filepath.Ext(subPath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(content)
}

// handleWorkflows renders a list of GitHub Actions workflows (.github/workflows).
func (s *Server) handleWorkflows(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		headRef, _, err := gitHead(s.repoPath)
		if err != nil {
			s.httpError(w, r, http.StatusInternalServerError, "Failed to read HEAD", err)
			return
		}
		ref = headRef
	}

	base, err := s.baseData(ref)
	if err != nil {
		s.httpError(w, r, http.StatusInternalServerError, "Failed to load repo metadata", err)
		return
	}

	paths, err := gitLsWorkflows(s.repoPath, ref)
	if err != nil {
		s.httpError(w, r, http.StatusInternalServerError, "Failed to list workflows", err)
		return
	}

	data := WorkflowsData{
		BaseData:  base,
		Ref:       ref,
		Workflows: paths,
	}

	t, ok := s.tmpls["workflows"]
	if !ok {
		log.Printf("template not found: workflows")
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "workflows", data); err != nil {
		log.Printf("render workflows: %v", err)
	}
}

// baseData builds BaseData for a given ref.
func (s *Server) baseData(ref string) (BaseData, error) {
	branches, err := gitBranches(s.repoPath)
	if err != nil {
		return BaseData{}, err
	}
	hasPages, err := gitHasBranch(s.repoPath, "gh-pages")
	if err != nil {
		return BaseData{}, err
	}

	return BaseData{
		RepoName:      s.repoName,
		Ref:           ref,
		Branches:      branches,
		HasGHPages:    hasPages,
		PagesBranches: branches, // All branches can be viewed as pages
	}, nil
}

// httpError logs and sends an HTTP error response.
func (s *Server) httpError(w http.ResponseWriter, r *http.Request, status int, msg string, err error) {
	if err != nil {
		log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
	}
	http.Error(w, msg, status)
}

// loggingMiddleware logs basic request information.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, lw.status, time.Since(start))
	})
}

// loggingResponseWriter wraps http.ResponseWriter to capture status codes.
type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader captures the status code before writing headers.
func (w *loggingResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// normalizeRepoPath converts backslashes to forward slashes and trims leading slashes.
func normalizeRepoPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "/")
	return p
}

// parentPath returns the parent path of a repository path.
func parentPath(p string) string {
	if p == "" {
		return ""
	}
	parts := strings.Split(p, "/")
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[:len(parts)-1], "/")
}

// runGit executes a git command in the given repository and returns stdout as a string.
func runGit(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %v: %w", args, err)
	}
	return string(out), nil
}

// runGitRaw executes a git command and returns stdout as bytes.
func runGitRaw(repoPath string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %v: %w", args, err)
	}
	return out, nil
}

// gitHead returns the current HEAD ref name (branch or "HEAD") and short hash.
func gitHead(repoPath string) (string, string, error) {
	ref, err := runGit(repoPath, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		// Detached HEAD; use "HEAD" as pseudo ref.
		ref = "HEAD"
	} else {
		ref = strings.TrimSpace(ref)
	}
	hash, err := runGit(repoPath, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", "", err
	}
	return ref, strings.TrimSpace(hash), nil
}

// gitBranches returns a list of local branch names.
func gitBranches(repoPath string) ([]string, error) {
	out, err := runGit(repoPath, "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var branches []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

// gitHasBranch reports whether the given branch exists.
func gitHasBranch(repoPath, name string) (bool, error) {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	cmd.Dir = repoPath
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

// gitLsTree lists entries in a given tree at ref/path.
func gitLsTree(repoPath, ref, path string) ([]TreeEntry, error) {
	args := []string{"ls-tree", "-z", "-l", ref}
	if path != "" {
		args = append(args, "--", path)
	}
	out, err := runGitRaw(repoPath, args...)
	if err != nil {
		return nil, err
	}
	raw := string(out)
	if raw == "" {
		return nil, nil
	}
	records := strings.Split(raw, "\x00")
	var entries []TreeEntry
	for _, rec := range records {
		if rec == "" {
			continue
		}
		// Format: "<mode> <type> <object> <size>\t<name>\n"
		parts := strings.SplitN(rec, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		meta := parts[0]
		name := strings.TrimSpace(parts[1])
		metaParts := strings.Fields(meta)
		if len(metaParts) < 4 {
			continue
		}
		mode := metaParts[0]
		typ := metaParts[1]
		sizeStr := metaParts[3]
		var sz int64
		if sizeStr != "-" {
			if v, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
				sz = v
			}
		}
		entry := TreeEntry{
			Name: name,
			Mode: mode,
			Type: typ,
			Size: sz,
		}
		entries = append(entries, entry)
	}

	// Directories first, then files, simple stable-ish ordering by name.
	var dirs, files []TreeEntry
	for _, e := range entries {
		if e.Type == "tree" {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}
	return append(dirs, files...), nil
}

// gitShowFile returns the content of ref:path from the repository.
func gitShowFile(repoPath, spec string) ([]byte, error) {
	return runGitRaw(repoPath, "show", spec)
}

// gitLog returns a short log for a given ref, limited to n commits.
func gitLog(repoPath, ref string, n int) ([]Commit, error) {
	format := "%h%x09%ad%x09%s"
	out, err := runGit(repoPath, "log", "--date=short", fmt.Sprintf("-n%d", n), "--pretty=format:"+format, ref)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var commits []Commit
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		commits = append(commits, Commit{
			Hash:    parts[0],
			Date:    parts[1],
			Subject: parts[2],
		})
	}
	return commits, nil
}

// gitDiff returns a unified diff between from and to.
func gitDiff(repoPath, from, to string) (string, error) {
	out, err := runGit(repoPath, "diff", "--stat", "--patch", from, to)
	if err != nil {
		return "", err
	}
	if len(out) == 0 {
		return "No differences.\n", nil
	}
	return out, nil
}

// gitLsWorkflows lists files under .github/workflows at the given ref.
func gitLsWorkflows(repoPath, ref string) ([]string, error) {
	out, err := runGit(repoPath, "ls-tree", "--name-only", "-z", ref, "--", ".github/workflows")
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	records := strings.Split(out, "\x00")
	var paths []string
	for _, rec := range records {
		rec = strings.TrimSpace(rec)
		if rec != "" {
			paths = append(paths, rec)
		}
	}
	return paths, nil
}

// handleAppCSS serves the bundled CSS.
func handleAppCSS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write([]byte(appCSSContent))
}

// handleAppJS serves the bundled JavaScript.
func handleAppJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(appJSContent))
}

func init() {
	// Optional: ensure mime has common types for gh-pages-like sites.
	_ = mime.AddExtensionType(".js", "application/javascript")
	_ = mime.AddExtensionType(".css", "text/css")
	_ = mime.AddExtensionType(".html", "text/html; charset=utf-8")
}
