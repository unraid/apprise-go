package notify

// Generated from upstream Apprise 1.12.0's html_to_markdown.
// Regenerate rather than editing an expectation by hand.

var htmlToMarkdownCorpus = []struct{ name, input, want string }{
	{"blockquote", "<blockquote>quoted</blockquote>", "> quoted"},
	{"bold", "<b>This is Bold Text</b>", "**This is Bold Text**"},
	{"break", "line<br/>next", "line  \nnext"},
	{"code", "<code>x = 1</code>", "`x = 1`"},
	{"em", "<em>em</em>", "*em*"},
	{"entities", "<p>a &amp; b &lt; c</p>", "a & b \\< c"},
	{"heading", "<h1>Title</h1><h2>Sub</h2>", "# Title\n## Sub"},
	{"hr", "before<hr/>after", "before\n\n---\nafter"},
	{"image", "<img src=\"https://example.com/a.png\" alt=\"alt\"/>", ""},
	{"italic", "<i>italic</i>", "*italic*"},
	{"link", "<a href=\"https://example.com\">example</a>", "[example](<https://example.com>)"},
	{"mixed", "<p>Hello <b>bold</b> and <i>italic</i> with <a href='https://x.io'>link</a></p>", "Hello **bold** and *italic* with [link](<https://x.io>)"},
	{"nested_list", "<ul><li>a<ul><li>b</li></ul></li></ul>", "- a\n  - b"},
	{"ol", "<ol><li>a</li><li>b</li></ol>", "1. a\n2. b"},
	{"paragraphs", "<p>one</p><p>two</p>", "one\n\ntwo"},
	{"plain", "no markup here", "no markup here"},
	{"pre", "<pre>raw  text</pre>", "```\nraw  text\n```"},
	{"strong", "<strong>strong</strong>", "**strong**"},
	{"ul", "<ul><li>a</li><li>b</li></ul>", "- a\n- b"},
}
