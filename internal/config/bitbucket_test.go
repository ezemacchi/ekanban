package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// The token is sent to the server's address, so only config.toml names it. A
// repository file that tried would otherwise collect the reader's token.
func TestRepoFileCannotRedirectTheBitbucketToken(t *testing.T) {
	write(t, `[bitbucket]
url = "https://bitbucket.mine.example"
token_env = "MY_BB_TOKEN"
`)
	main, wt := fakeRepo(t)
	put(t, filepath.Join(main, RepoFileName), `[bitbucket]
url = "https://evil.example"
token_env = "STEAL_ME"
`)
	s := LoadFor(wt)
	if s.Bitbucket.URL != "https://bitbucket.mine.example" || s.Bitbucket.TokenEnv != "MY_BB_TOKEN" {
		t.Fatalf("the repository file chose the server or the token: %+v", s.Bitbucket)
	}
	var said bool
	for _, p := range s.Problems {
		said = said || strings.Contains(p, "[bitbucket] url and token_env")
	}
	if !said {
		t.Fatalf("no word about what was ignored: %q", s.Problems)
	}
}

func TestBitbucketIsOffWithoutAURLAndDefaultsTheTokenVariable(t *testing.T) {
	write(t, "")
	s := Load()
	if s.Bitbucket.URL != "" || s.Bitbucket.TokenEnv != "BITBUCKET_TOKEN" {
		t.Fatalf("%+v", s.Bitbucket)
	}
}
