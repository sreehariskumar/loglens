package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type App struct{ root string }
type Entry struct {
	Line  int    `json:"line"`
	Time  string `json:"time,omitempty"`
	Level string `json:"level"`
	Text  string `json:"text"`
}
type SearchResponse struct {
	File      string  `json:"file"`
	Total     int     `json:"total"`
	Matched   int     `json:"matched"`
	Entries   []Entry `json:"entries"`
	Truncated bool    `json:"truncated"`
}

var timePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?`),
	regexp.MustCompile(`\d{4}/\d{2}/\d{2}[ T]\d{2}:\d{2}:\d{2}`),
}
var levelRE = regexp.MustCompile(`(?i)\b(TRACE|DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL|CRITICAL)\b`)

func main() {
	root := flag.String("root", ".", "directory containing log files")
	addr := flag.String("listen", "127.0.0.1:8080", "HTTP listen address")
	flag.Parse()
	abs, err := filepath.Abs(*root)
	if err != nil {
		log.Fatal(err)
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		log.Fatalf("invalid root directory: %s", abs)
	}
	app := &App{root: abs}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/files", app.files)
	mux.HandleFunc("/api/search", app.search)
	mux.Handle("/", http.FileServer(http.Dir("web")))
	log.Printf("LogLens serving %s (log root: %s)", *addr, abs)
	log.Fatal(http.ListenAndServe(*addr, securityHeaders(mux)))
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
func (a *App) safePath(name string) (string, error) {
	if name == "" || filepath.IsAbs(name) {
		return "", fmt.Errorf("invalid file path")
	}
	p := filepath.Clean(filepath.Join(a.root, name))
	rel, err := filepath.Rel(a.root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path is outside log root")
	}
	// Resolve symlinks before opening: lexical containment alone is not enough.
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	rel, err = filepath.Rel(a.root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("resolved file is outside log root")
	}
	return resolved, nil
}
func (a *App) files(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	var out []string
	err := filepath.Walk(a.root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if p != a.root && strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode().IsRegular() {
			ext := strings.ToLower(filepath.Ext(info.Name()))
			if ext == ".log" || ext == ".txt" || ext == ".json" || ext == ".jsonl" || ext == "" {
				rel, _ := filepath.Rel(a.root, p)
				out = append(out, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	sort.Strings(out)
	writeJSON(w, out)
}
func (a *App) search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	q := r.URL.Query()
	name := q.Get("file")
	p, err := a.safePath(name)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "not a regular file", 400)
		return
	}
	keyword := strings.TrimSpace(q.Get("q"))
	regexMode := q.Get("regex") == "true"
	var matcher *regexp.Regexp
	if keyword != "" && regexMode {
		matcher, err = regexp.Compile(keyword)
		if err != nil {
			http.Error(w, "invalid regex: "+err.Error(), 400)
			return
		}
	}
	level := strings.ToUpper(q.Get("level"))
	from, to := q.Get("from"), q.Get("to")
	fromTime, fromOK := parseTime(from)
	toTime, toOK := parseTime(to)
	limit := 500
	if n, e := strconv.Atoi(q.Get("limit")); e == nil && n > 0 && n <= 5000 {
		limit = n
	}
	resp := SearchResponse{File: name, Entries: make([]Entry, 0)}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		resp.Total++
		line := sc.Text()
		if keyword != "" {
			if regexMode {
				if !matcher.MatchString(line) {
					continue
				}
			} else if !strings.Contains(strings.ToLower(line), strings.ToLower(keyword)) {
				continue
			}
		}
		lv := ""
		if m := levelRE.FindStringSubmatch(line); len(m) > 1 {
			lv = strings.ToUpper(m[1])
			if lv == "WARNING" {
				lv = "WARN"
			}
			if lv == "CRITICAL" {
				lv = "FATAL"
			}
		}
		if level != "" && level != "ALL" && lv != level {
			continue
		}
		ts := extractTime(line)
		if fromOK || toOK {
			t, ok := parseTime(ts)
			if !ok {
				continue
			}
			if fromOK && t.Before(fromTime) {
				continue
			}
			if toOK && t.After(toTime) {
				continue
			}
		}
		resp.Matched++
		if len(resp.Entries) < limit {
			resp.Entries = append(resp.Entries, Entry{Line: resp.Total, Time: ts, Level: lv, Text: line})
		} else {
			resp.Truncated = true
		}
	}
	if err := sc.Err(); err != nil {
		http.Error(w, "read error: "+err.Error(), 500)
		return
	}
	writeJSON(w, resp)
}
func extractTime(s string) string {
	for _, re := range timePatterns {
		if x := re.FindString(s); x != "" {
			return x
		}
	}
	return ""
}
func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05", "2006/01/02 15:04:05"}
	for _, l := range layouts {
		if t, e := time.Parse(l, s); e == nil {
			return t, true
		}
	}
	// A timestamp without an explicit zone is interpreted as local server time.
	for _, l := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
		if t, e := time.ParseInLocation(l, s, time.Local); e == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
