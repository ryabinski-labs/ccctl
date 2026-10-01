package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestStaticServesPageButNotMissingFiles(t *testing.T) {
	s := &Server{Static: fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>ccctl</title>")},
		"assets/app.js": {Data: []byte("1")},
	}}
	for path, want := range map[string]int{
		"/":              http.StatusOK,
		"/assets/app.js": http.StatusOK,
		"/some/page":     http.StatusOK,
		"/robots.txt":    http.StatusNotFound,
		"/sitemap.xml":   http.StatusNotFound,
		"/assets/old.js": http.StatusNotFound,
	} {
		rec := httptest.NewRecorder()
		s.static(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, want)
		}
	}
}
