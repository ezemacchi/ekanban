package pipeline

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Instance is a running environment people test on, such as QA, whose version
// is read from its own address. A publish does not say what an instance runs:
// some instances are upgraded by hand, long after the publish.
type Instance struct {
	Name string // shown on the card: "qa"
	URL  string
	// Version's first group is the running version. It is matched against the
	// redirect's Location header first, then the body.
	Version *regexp.Regexp
	// Tag is the git tag of a version, with {version} in it: "v{version}".
	Tag string
}

// NewInstance checks [pipeline.qa]. No url is no instance; a bad one is
// reported and left out, so the board keeps the publish rule.
func NewInstance(name, url, version, tag string) (*Instance, []string) {
	if strings.TrimSpace(url) == "" {
		if version != "" || tag != "" {
			return nil, []string{"[pipeline.qa] has no url — no QA instance is read"}
		}
		return nil, nil
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, []string{fmt.Sprintf("[pipeline.qa] url %q is not http or https — no QA instance is read", url)}
	}
	if version == "" {
		return nil, []string{"[pipeline.qa] has no version pattern — no QA instance is read"}
	}
	re, err := regexp.Compile(version)
	if err != nil {
		return nil, []string{fmt.Sprintf("[pipeline.qa] version %q: %v — no QA instance is read", version, err)}
	}
	if re.NumSubexp() < 1 {
		return nil, []string{fmt.Sprintf("[pipeline.qa] version %q has no group for the version — no QA instance is read", version)}
	}
	if name == "" {
		name = "qa"
	}
	if tag == "" {
		tag = "v{version}"
	}
	if !strings.Contains(tag, "{version}") {
		return nil, []string{fmt.Sprintf("[pipeline.qa] tag %q has no {version} — no QA instance is read", tag)}
	}
	return &Instance{Name: name, URL: url, Version: re, Tag: tag}, nil
}

// InstanceState is what was last read from an instance.
type InstanceState struct {
	Version string
	SHA     string // the commit the version's tag points at; "" when unknown
	Problem string // why the version or its commit is unknown, or ""
}

const maxInstanceBody = 256 << 10

// readVersion asks the instance for its version without following redirects:
// the version is often only in the Location of the first answer.
func readVersion(ctx context.Context, client *http.Client, in *Instance) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if m := in.Version.FindStringSubmatch(resp.Header.Get("Location")); len(m) > 1 && m[1] != "" {
		return m[1], nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxInstanceBody))
	if err != nil {
		return "", err
	}
	if m := in.Version.FindSubmatch(body); len(m) > 1 && len(m[1]) > 0 {
		return string(m[1]), nil
	}
	return "", fmt.Errorf("no version in its answer (HTTP %d)", resp.StatusCode)
}

func instanceClient() *http.Client {
	return &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// readInstance reads the instance's version and the commit of its tag.
func (s *Source) readInstance(ctx context.Context) InstanceState {
	in := s.set.QA
	version, err := readVersion(ctx, instanceClient(), in)
	if err != nil {
		return InstanceState{Problem: fmt.Sprintf("%s version unknown: %v", in.Name, err)}
	}
	st := InstanceState{Version: version}
	tag := strings.ReplaceAll(in.Tag, "{version}", version)
	ref := "refs/tags/" + tag
	st.SHA = s.git(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if st.SHA == "" {
		s.git(ctx, "fetch", "--quiet", "origin", ref+":"+ref)
		st.SHA = s.git(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	}
	if st.SHA == "" {
		st.Problem = fmt.Sprintf("%s runs %s, but there is no tag %s", in.Name, version, tag)
	}
	return st
}

// QAState is what was last read from the QA instance; zero when none is set.
func (s *Source) QAState() InstanceState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.qa
}
