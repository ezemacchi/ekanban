package links

import "testing"

// A question keeps its words; its links come out with their labels, a bare
// URL labelled by its host.
func TestInTextSplitsWordsFromLinks(t *testing.T) {
	text, found := InText("Product (Ana): which title? [Chat with Ana](https://chat.example.com/l/chat?users=a&message=Hi%20Ana) or see https://wiki.example.com/page.")
	if text != "Product (Ana): which title? or see ." {
		t.Fatalf("text %q", text)
	}
	want := []Link{
		{Label: "Chat with Ana", URL: "https://chat.example.com/l/chat?users=a&message=Hi%20Ana"},
		{Label: "wiki.example.com", URL: "https://wiki.example.com/page"},
	}
	if len(found) != len(want) || found[0] != want[0] || found[1] != want[1] {
		t.Fatalf("links %+v", found)
	}
	if text, found := InText("no links here"); text != "no links here" || found != nil {
		t.Fatalf("%q %v", text, found)
	}
}

// A link to a question's brief is neither words nor a chat: Files finds it.
func TestBriefLinkIsAFileNotAChat(t *testing.T) {
	line := "Product (Ana): which title? [brief](questions/which-title.md) [Chat with Ana](https://chat.example.com/x)"
	text, found := InText(line)
	if text != "Product (Ana): which title?" || len(found) != 1 || found[0].Label != "Chat with Ana" {
		t.Fatalf("%q %+v", text, found)
	}
	files := Files(line)
	if len(files) != 1 || files[0] != (Link{Label: "brief", URL: "questions/which-title.md"}) {
		t.Fatalf("files %+v", files)
	}
	if Files("see [the docs](https://example.com/a.md)") != nil {
		t.Fatal("a web page ending in .md was taken for a file")
	}
}
