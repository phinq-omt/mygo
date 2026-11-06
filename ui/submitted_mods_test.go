package ui

import "testing"

// SubmittedMods tells Enter from Shift+Enter in a single-line field.
func TestSubmittedMods(t *testing.T) {
	var (
		text string
		got  []Modifiers
	)
	tt := NewTester(func(c *Context) {
		in := TextInputBase(c, &text).Width(200).AutoFocus()
		if mods, ok := in.SubmittedMods(); ok {
			got = append(got, mods)
		}
	}, 300, 100)
	tt.Frame()
	tt.Type("x")
	tt.Key(0, KeyEnter)
	tt.Frame()
	tt.Key(Shift, KeyEnter)
	tt.Frame()
	if len(got) != 2 || got[0]&Shift != 0 || got[1]&Shift == 0 {
		t.Errorf("submissions %v, want Enter then Shift+Enter", got)
	}
}
