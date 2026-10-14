package pipeline

import (
	"testing"

	"github.com/ezemacchi/ekanban/internal/ticket"
)

// The run's open questions reach the board with the rest of its state.
func TestDescribeRunCarriesTheQuestions(t *testing.T) {
	run := &ticket.Run{Questions: []ticket.Question{{Line: "Which caption?"}}}
	var info Info
	describeRun(&info, run)
	if len(info.Questions) != 1 || info.Questions[0].Line != "Which caption?" {
		t.Fatalf("questions %+v", info.Questions)
	}
	run.Questions[0].Line = "changed"
	if info.Questions[0].Line != "Which caption?" {
		t.Fatal("the board shares the run's slice")
	}
}
