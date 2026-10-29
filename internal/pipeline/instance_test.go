package pipeline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mustInstance(t *testing.T, url, version string) *Instance {
	t.Helper()
	in, problems := NewInstance("", url, version, "")
	if len(problems) > 0 || in == nil {
		t.Fatalf("NewInstance: %v", problems)
	}
	return in
}

func TestNewInstanceDefaults(t *testing.T) {
	in := mustInstance(t, "https://qa.example.com/", `[?&]v=([^&#]+)`)
	if in.Name != "qa" || in.Tag != "v{version}" {
		t.Fatalf("defaults: %+v", in)
	}
}

func TestNewInstanceRejectsBadSettings(t *testing.T) {
	cases := []struct {
		name, url, version, tag, want string
	}{
		{"version without url", "", `v=(.+)`, "", "no url"},
		{"not http", "ftp://qa.example.com/", `v=(.+)`, "", "not http"},
		{"no version", "https://qa.example.com/", "", "", "no version"},
		{"bad regexp", "https://qa.example.com/", `v=(`, "", "version"},
		{"no group", "https://qa.example.com/", `v=.+`, "", "no group"},
		{"tag without placeholder", "https://qa.example.com/", `v=(.+)`, "release", "{version}"},
	}
	for _, c := range cases {
		in, problems := NewInstance("qa", c.url, c.version, c.tag)
		if in != nil || len(problems) != 1 || !strings.Contains(problems[0], c.want) {
			t.Errorf("%s: instance %v, problems %v", c.name, in, problems)
		}
	}
	if in, problems := NewInstance("", "", "", ""); in != nil || problems != nil {
		t.Errorf("nothing set must be no instance and no problem: %v %v", in, problems)
	}
}

func TestReadVersionFromRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "" {
			t.Error("the redirect was followed")
		}
		http.Redirect(w, r, "/?v=1.2.3", http.StatusFound)
	}))
	defer srv.Close()
	got, err := readVersion(context.Background(), instanceClient(), mustInstance(t, srv.URL+"/", `[?&]v=([^&#]+)`))
	if err != nil || got != "1.2.3" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestReadVersionFromBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<meta name="version" content="2.0.1">`))
	}))
	defer srv.Close()
	got, err := readVersion(context.Background(), instanceClient(), mustInstance(t, srv.URL, `name="version" content="([^"]+)"`))
	if err != nil || got != "2.0.1" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestReadVersionMissingIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()
	if got, err := readVersion(context.Background(), instanceClient(), mustInstance(t, srv.URL, `v=([0-9.]+)`)); err == nil {
		t.Fatalf("expected an error, got %q", got)
	}
}
