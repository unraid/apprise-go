package notify

import "testing"

// The corpus records what upstream's converter produces. A mismatch means our
// output diverged, not that the expectation is wrong: regenerate the corpus
// from upstream rather than editing a string here.
func TestHTMLToMarkdownMatchesUpstream(t *testing.T) {
	for _, testCase := range htmlToMarkdownCorpus {
		t.Run(testCase.name, func(t *testing.T) {
			if got := htmlToMarkdown(testCase.input); got != testCase.want {
				t.Fatalf("got %q want %q", got, testCase.want)
			}
		})
	}
}
