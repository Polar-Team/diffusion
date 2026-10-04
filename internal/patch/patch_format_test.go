package patch

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// parseSeqDoc parses a tasks-file-shaped YAML document (a top-level
// sequence) for marshalTaskDoc round-trip tests.
func parseSeqDoc(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return &doc
}

// TestMarshalTaskDocBlankLinesBetweenTopLevelItems covers (a): three tasks
// round-trip with exactly one blank line between consecutive items, none
// before the first and none after the last.
func TestMarshalTaskDocBlankLinesBetweenTopLevelItems(t *testing.T) {
	doc := parseSeqDoc(t, `---
- name: Task1
  debug:
    msg: hi
- name: Task2
  debug:
    msg: there
- name: Task3
  debug:
    msg: done
`)
	out, err := marshalTaskDoc(doc, false)
	if err != nil {
		t.Fatalf("marshalTaskDoc: %v", err)
	}
	s := string(out)

	if strings.HasPrefix(s, "\n") {
		t.Fatalf("blank line before first item:\n%s", s)
	}
	if strings.HasSuffix(s, "\n\n") {
		t.Fatalf("blank line after last item:\n%s", s)
	}
	lines := strings.Split(s, "\n")
	var blankCount int
	for _, l := range lines {
		if l == "" {
			blankCount++
		}
	}
	// Exactly 2 blank-line separators, plus the single trailing "" from
	// the final "\n" that strings.Split always produces.
	if blankCount != 3 {
		t.Fatalf("blank line count = %d, want 3 (2 separators + 1 trailing split artifact):\n%s", blankCount, s)
	}
	if !strings.Contains(s, "msg: hi\n\n- name: Task2") {
		t.Errorf("missing blank line between Task1 and Task2:\n%s", s)
	}
	if !strings.Contains(s, "msg: there\n\n- name: Task3") {
		t.Errorf("missing blank line between Task2 and Task3:\n%s", s)
	}
}

// TestMarshalTaskDocHeadCommentStaysWithItem covers (b): a HeadComment
// attached to an item must keep the blank line ABOVE the comment, not
// between the comment and the "- ".
func TestMarshalTaskDocHeadCommentStaysWithItem(t *testing.T) {
	doc := parseSeqDoc(t, `---
- name: Task1
  debug:
    msg: hi
# head comment for task 2
- name: Task2
  debug:
    msg: there
`)
	out, err := marshalTaskDoc(doc, false)
	if err != nil {
		t.Fatalf("marshalTaskDoc: %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "msg: hi\n\n# head comment for task 2\n- name: Task2") {
		t.Errorf("blank line must sit above the head comment block, not between the comment and the item:\n%s", s)
	}
	if strings.Contains(s, "# head comment for task 2\n\n- name: Task2") {
		t.Errorf("blank line must not be inserted between the comment and its item:\n%s", s)
	}
}

// TestMarshalTaskDocNestedBlockUntouched covers (c): a block/rescue/always
// child sequence must never get blank lines inserted between its items,
// even though those children are themselves yaml.Node SequenceNodes.
func TestMarshalTaskDocNestedBlockUntouched(t *testing.T) {
	doc := parseSeqDoc(t, `---
- name: Outer block
  block:
    - name: Inner1
      debug:
        msg: one
    - name: Inner2
      debug:
        msg: two
    - name: Inner3
      debug:
        msg: three
- name: Task2
  debug:
    msg: after
`)
	out, err := marshalTaskDoc(doc, false)
	if err != nil {
		t.Fatalf("marshalTaskDoc: %v", err)
	}
	s := string(out)

	for _, want := range []string{
		"msg: one\n    - name: Inner2",
		"msg: two\n    - name: Inner3",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("nested block items must stay adjacent (no blank line), missing %q in:\n%s", want, s)
		}
	}
	// The outer top-level boundary (block task -> Task2) still gets one.
	if !strings.Contains(s, "msg: three\n\n- name: Task2") {
		t.Errorf("missing blank line at the top-level boundary after the block:\n%s", s)
	}
	// No blank line was introduced anywhere inside the nested sequence.
	if strings.Contains(s, "msg: one\n\n    - name: Inner2") ||
		strings.Contains(s, "msg: two\n\n    - name: Inner3") {
		t.Errorf("blank line leaked into nested block sequence:\n%s", s)
	}
}

// TestMarshalTaskDocPreservesDocStart covers (d): the "---" marker is
// restored only when the original file had one.
func TestMarshalTaskDocPreservesDocStart(t *testing.T) {
	doc := parseSeqDoc(t, `- name: Task1
  debug:
    msg: hi
`)

	withStart, err := marshalTaskDoc(doc, true)
	if err != nil {
		t.Fatalf("marshalTaskDoc: %v", err)
	}
	if !strings.HasPrefix(string(withStart), "---\n") {
		t.Errorf("expected '---' header, got:\n%s", withStart)
	}

	withoutStart, err := marshalTaskDoc(doc, false)
	if err != nil {
		t.Fatalf("marshalTaskDoc: %v", err)
	}
	if strings.HasPrefix(string(withoutStart), "---") {
		t.Errorf("did not expect '---' header, got:\n%s", withoutStart)
	}
}

// TestMarshalTaskDocSingleTrailingNewline covers item 3: output always
// ends with exactly one trailing newline, never zero and never several.
func TestMarshalTaskDocSingleTrailingNewline(t *testing.T) {
	doc := parseSeqDoc(t, `---
- name: Task1
  debug:
    msg: hi
- name: Task2
  debug:
    msg: there
`)
	out, err := marshalTaskDoc(doc, true)
	if err != nil {
		t.Fatalf("marshalTaskDoc: %v", err)
	}
	if strings.HasSuffix(string(out), "\n\n") {
		t.Fatalf("more than one trailing newline:\n%q", out)
	}
	if !strings.HasSuffix(string(out), "\n") {
		t.Fatalf("missing trailing newline:\n%q", out)
	}
}

// TestHasDocStart covers the detection helper directly: present, absent,
// and tolerant of leading blank lines / CRLF.
func TestHasDocStart(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{"present", "---\n- name: a\n", true},
		{"present with trailing content on marker line", "--- \n- name: a\n", true},
		{"absent", "- name: a\n", false},
		{"present after leading blank lines", "\n\n---\n- name: a\n", true},
		{"absent, leading blank lines only", "\n\n- name: a\n", false},
		{"present, CRLF line endings", "---\r\n- name: a\r\n", true},
		{"empty file", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasDocStart([]byte(tt.src)); got != tt.want {
				t.Errorf("hasDocStart(%q) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

// TestSpaceTopLevelItemsNoDoubleBlank verifies that a line already blank
// above an item boundary is not doubled.
func TestSpaceTopLevelItemsNoDoubleBlank(t *testing.T) {
	in := "- name: Task1\n  debug:\n    msg: hi\n\n- name: Task2\n  debug:\n    msg: there\n"
	out := string(spaceTopLevelItems([]byte(in)))
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("blank line was doubled:\n%s", out)
	}
	if !strings.Contains(out, "msg: hi\n\n- name: Task2") {
		t.Errorf("expected single blank line preserved:\n%s", out)
	}
}

// TestSpaceTopLevelItemsSingleItemNoop verifies a single-item sequence is
// left untouched (no blank lines to insert at all).
func TestSpaceTopLevelItemsSingleItemNoop(t *testing.T) {
	in := "- name: Task1\n  debug:\n    msg: hi\n"
	out := string(spaceTopLevelItems([]byte(in)))
	if out != in {
		t.Errorf("single item changed:\nin:  %q\nout: %q", in, out)
	}
}
