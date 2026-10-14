package bitbucket

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// dashboard answers the review list: pages[status] is each page's values,
// in order; a page that is not the last says where the next starts.
func dashboard(t *testing.T, pages map[string][]string, code int) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/latest/dashboard/pull-requests", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Header.Get("Authorization") != "Bearer "+secret || code != 0 {
			http.Error(w, "no", max(code, http.StatusUnauthorized))
			return
		}
		if q.Get("role") != "REVIEWER" || q.Get("state") != "OPEN" {
			t.Errorf("asked %v", q)
		}
		list := pages[q.Get("participantStatus")]
		page := 0
		_, _ = fmt.Sscan(q.Get("start"), &page)
		if page >= len(list) {
			_, _ = w.Write([]byte(`{"values":[],"isLastPage":true}`))
			return
		}
		last := page == len(list)-1
		_, _ = fmt.Fprintf(w, `{"values":[%s],"isLastPage":%t,"nextPageStart":%d}`, list[page], last, page+1)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("BB_TEST_TOKEN", secret)
	c, why := New(Config{URL: srv.URL, TokenEnv: "BB_TEST_TOKEN"})
	if c == nil {
		t.Fatalf("no client: %s", why)
	}
	c.http = srv.Client()
	return c
}

func pr(id int, project, repo string) string {
	return fmt.Sprintf(`{"id":%d,"title":"PROJ-%d Fix it","updatedDate":1700000000000,
		"fromRef":{"displayId":"feature/PROJ-%d"},
		"toRef":{"displayId":"main","repository":{"slug":%q,"project":{"key":%q}}},
		"author":{"user":{"displayName":"Ana","name":"ana"}},
		"links":{"self":[{"href":"https://bb.example/pr/%d"}]}}`, id, id, id, repo, project, id)
}

func TestAwaitingReviewKeepsTheBoardsRepositoryAndBothStatuses(t *testing.T) {
	c := dashboard(t, map[string][]string{
		"UNAPPROVED": {pr(1, "PROJ", "app") + "," + pr(2, "OTHER", "app"), pr(3, "proj", "APP")},
		"NEEDS_WORK": {pr(4, "PROJ", "app")},
	}, 0)
	got, err := c.AwaitingReview(context.Background(), "PROJ", "app")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, a := range got {
		ids = append(ids, fmt.Sprintf("%d:%s", a.Number, a.MyStatus))
	}
	if strings.Join(ids, " ") != "1:UNAPPROVED 3:UNAPPROVED 4:NEEDS_WORK" {
		t.Fatalf("got %v", ids)
	}
	a := got[0]
	if a.Title != "PROJ-1 Fix it" || a.Branch != "feature/PROJ-1" || a.Target != "main" || a.Author != "Ana" ||
		a.Link != "https://bb.example/pr/1" || a.Updated.IsZero() {
		t.Fatalf("fields: %+v", a)
	}
}

func TestAwaitingReviewSaysWhenTheTokenIsRefused(t *testing.T) {
	c := dashboard(t, nil, http.StatusUnauthorized)
	_, err := c.AwaitingReview(context.Background(), "PROJ", "app")
	if err == nil || !strings.Contains(err.Error(), "BB_TEST_TOKEN was refused") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("the token is in the message")
	}
}

func TestAwaitingReviewRefusesANameThatIsNotARepository(t *testing.T) {
	c := dashboard(t, nil, 0)
	if _, err := c.AwaitingReview(context.Background(), "PROJ", "../x"); err == nil {
		t.Fatal("a path was accepted as a repository")
	}
}
