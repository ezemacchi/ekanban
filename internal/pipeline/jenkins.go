package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func init() {
	RegisterCI("jenkins", func(c CIConfig) CI {
		return &jenkins{prJobs: c.URL, http: &http.Client{Timeout: 20 * time.Second}}
	})
}

// jenkins reads a multibranch job whose children are PR-<n>, and publish jobs,
// through the JSON API, which answers without credentials.
type jenkins struct {
	prJobs string
	http   *http.Client
}

func (j *jenkins) Name() string { return "Jenkins" }

type jenkinsRevision struct {
	LastBuiltRevision *struct {
		SHA1   string `json:"SHA1"`
		Branch []struct {
			Name string `json:"name"`
		} `json:"branch"`
	} `json:"lastBuiltRevision"`
}

func revisionOf(actions []jenkinsRevision) (sha, branch string) {
	for _, a := range actions {
		if a.LastBuiltRevision != nil {
			sha = a.LastBuiltRevision.SHA1
			if len(a.LastBuiltRevision.Branch) > 0 {
				branch = strings.TrimPrefix(a.LastBuiltRevision.Branch[0].Name, "origin/")
			}
			return
		}
	}
	return
}

func (j *jenkins) PRBuilds(ctx context.Context) ([]PRBuild, error) {
	if j.prJobs == "" {
		return nil, nil
	}
	var body struct {
		Jobs []struct {
			Name      string `json:"name"`
			Color     string `json:"color"`
			LastBuild *struct {
				Result   string            `json:"result"`
				Building bool              `json:"building"`
				Actions  []jenkinsRevision `json:"actions"`
			} `json:"lastBuild"`
		} `json:"jobs"`
	}
	tree := "jobs[name,color,lastBuild[result,building,actions[lastBuiltRevision[SHA1,branch[name]]]]]"
	if err := j.getJSON(ctx, strings.TrimRight(j.prJobs, "/")+"/api/json?tree="+tree, &body); err != nil {
		return nil, err
	}
	var builds []PRBuild
	for _, job := range body.Jobs {
		n, err := strconv.Atoi(strings.TrimPrefix(job.Name, "PR-"))
		if err != nil || !strings.HasPrefix(job.Name, "PR-") {
			continue
		}
		b := PRBuild{Number: n, Open: job.Color != "disabled"}
		if job.LastBuild != nil {
			b.Result = job.LastBuild.Result
			if job.LastBuild.Building {
				b.Result = "RUNNING"
			}
			b.SHA, b.Branch = revisionOf(job.LastBuild.Actions)
		}
		builds = append(builds, b)
	}
	return builds, nil
}

func (j *jenkins) LastPublish(ctx context.Context, t Target) (Publish, error) {
	var body struct {
		Builds []struct {
			Number  int               `json:"number"`
			Result  string            `json:"result"`
			Actions []jenkinsRevision `json:"actions"`
		} `json:"builds"`
	}
	tree := "builds[number,result,actions[lastBuiltRevision[SHA1]]]{0,6}"
	if err := j.getJSON(ctx, strings.TrimRight(t.Publish, "/")+"/api/json?tree="+tree, &body); err != nil {
		return Publish{}, err
	}
	for _, b := range body.Builds {
		if b.Result == "SUCCESS" {
			sha, _ := revisionOf(b.Actions)
			return Publish{Number: b.Number, SHA: sha}, nil
		}
	}
	return Publish{}, fmt.Errorf("no successful publish")
}

func (j *jenkins) getJSON(ctx context.Context, url string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := j.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from Jenkins", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}
