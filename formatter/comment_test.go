package formatter

import (
	"testing"
)

// TestComment covers the fields the printer places a comment by. The AST holds
// no comments and the printer writes one out before the first statement that
// starts below its line; that makes Line the field everything depends on.
func TestComment(t *testing.T) {
	t.Parallel()
	comments := CollectComments("<?php\n// first\n$a = 1;\n/* second */\n$b = 2;\n")
	if len(comments) != 2 {
		t.Fatalf("CollectComments found %d comments, want 2: %+v", len(comments), comments)
	}
	if comments[0].Line >= comments[1].Line {
		t.Errorf("the comments are not in source order: %d then %d", comments[0].Line, comments[1].Line)
	}
	for _, comment := range comments {
		if comment.Text == "" {
			t.Errorf("a comment carries no text: %+v", comment)
		}
		if comment.Line == 0 {
			t.Errorf("a comment carries no line, so the printer cannot place it: %+v", comment)
		}
	}

	if got := CollectComments("<?php $a = 1;"); len(got) != 0 {
		t.Errorf("CollectComments found %d comments in source holding none", len(got))
	}
}
