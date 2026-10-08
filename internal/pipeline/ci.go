package pipeline

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// CI is a continuous integration server: where pull request builds and
// publishes are read from. Jenkins (jenkins.go) is the built-in one; another
// server is an implementation plus a RegisterCI call, chosen by
// [pipeline.ci] kind in config.toml.
type CI interface {
	// Name is how the board names it: "Jenkins green", "Jenkins is not answering".
	Name() string
	// PRBuilds are the last builds of the pull requests the server knows.
	PRBuilds(ctx context.Context) ([]PRBuild, error)
	// LastPublish is the newest successful publish of target, read from
	// target.Publish (a job URL for Jenkins).
	LastPublish(ctx context.Context, target Target) (Publish, error)
}

// PRBuild is the last build of one pull request.
type PRBuild struct {
	Number int
	Open   bool   // the pull request is still open
	Result string // SUCCESS, FAILURE, UNSTABLE, RUNNING, ...
	Branch string // the source branch, when the server reports it
	SHA    string // the commit built
}

// Publish is a successful deploy of a target branch.
type Publish struct {
	Number int
	SHA    string
}

// CIConfig is what config.toml says about the server.
type CIConfig struct {
	Kind string // registered name; "" or "none" is no server
	URL  string // the job whose children are the pull request builds
}

var (
	ciMu       sync.Mutex
	ciRegistry = map[string]func(CIConfig) CI{}
)

// RegisterCI makes a kind of server available to [pipeline.ci] kind.
func RegisterCI(kind string, build func(CIConfig) CI) {
	ciMu.Lock()
	defer ciMu.Unlock()
	if _, dup := ciRegistry[kind]; dup {
		panic("pipeline: CI " + kind + " registered twice")
	}
	ciRegistry[kind] = build
}

// NewCI builds the configured server; nil means none. An unknown kind is
// reported and treated as none.
func NewCI(c CIConfig) (CI, []string) {
	if c.Kind == "" || c.Kind == "none" {
		return nil, nil
	}
	ciMu.Lock()
	build, ok := ciRegistry[c.Kind]
	ciMu.Unlock()
	if !ok {
		return nil, []string{fmt.Sprintf("unknown CI kind %q (known: %v)", c.Kind, CIKinds())}
	}
	return build(c), nil
}

// CIKinds are the registered kinds, sorted.
func CIKinds() []string {
	ciMu.Lock()
	defer ciMu.Unlock()
	var out []string
	for k := range ciRegistry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
