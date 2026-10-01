package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func releaseServer(t *testing.T, gotAuth *[]string) *httptest.Server {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\n")
	_ = tw.WriteHeader(&tar.Header{Name: "ccctl", Mode: 0o755, Size: int64(len(body))})
	_, _ = tw.Write(body)
	_ = tw.Close()
	_ = gz.Close()

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotAuth = append(*gotAuth, r.Header.Get("Authorization"))
		if r.URL.Path == "/asset" {
			_, _ = w.Write(buf.Bytes())
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.1","assets":[{"name":"` + AssetName("linux", "amd64") + `","url":"` + srv.URL + `/asset"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func downloadOpts(srv *httptest.Server, env map[string]string, runErr error) *Options {
	return &Options{
		GOOS:    "linux",
		GOARCH:  "amd64",
		HTTP:    srv.Client(),
		APIBase: srv.URL,
		Env:     func(k string) string { return env[k] },
		Run: func(context.Context, string, ...string) ([]byte, error) {
			return nil, runErr
		},
	}
}

// The release repo is public: with no GITHUB_TOKEN and no `gh`, download must
// still work and must not send an Authorization header.
func TestDownloadAnonymousWhenNoToken(t *testing.T) {
	var auth []string
	srv := releaseServer(t, &auth)
	o := downloadOpts(srv, nil, errors.New("gh: not found"))
	dest := filepath.Join(t.TempDir(), "ccctl")

	tag, err := download(context.Background(), o, dest)
	if err != nil {
		t.Fatal(err)
	}
	if tag != "v0.1.1" {
		t.Fatalf("tag = %q", tag)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatal(err)
	}
	for _, a := range auth {
		if a != "" {
			t.Fatalf("anonymous download sent Authorization %q", a)
		}
	}
}

// A token, when present, is still used (it raises GitHub's API rate limit).
func TestDownloadSendsTokenWhenPresent(t *testing.T) {
	var auth []string
	srv := releaseServer(t, &auth)
	o := downloadOpts(srv, map[string]string{"GITHUB_TOKEN": "tok"}, nil)

	if _, err := download(context.Background(), o, filepath.Join(t.TempDir(), "ccctl")); err != nil {
		t.Fatal(err)
	}
	if len(auth) == 0 || auth[0] != "Bearer tok" {
		t.Fatalf("Authorization headers = %q", auth)
	}
}
