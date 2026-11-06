package terminal

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func selectionText(term *Terminal) string { return term.SelectedText() }

func TestFind(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 400, 200)
	tt.Frame()
	cols, _ := term.Size()
	_ = cols
	var b strings.Builder
	for i := range 500 {
		switch i {
		case 3:
			b.WriteString("an ERROR here\r\n")
		case 200:
			b.WriteString("日本語 error ✓\r\n")
		case 480:
			// A line the terminal wraps, with the match across the wrap.
			b.WriteString(strings.Repeat("x", cols-2) + "error tail\r\n")
		default:
			b.WriteString("line " + itoa(i) + "\r\n")
		}
	}
	term.Feed([]byte(b.String()))
	tt.Frame()

	got := term.Find("error", false)
	if len(got) != 3 {
		t.Fatalf("Find(error) = %v, want 3 matches", got)
	}
	if m := got[0]; m.Row != m.EndRow {
		t.Errorf("first match = %+v, want one row", m)
	}
	if m := got[2]; m.EndRow != m.Row+1 {
		t.Errorf("wrapped match = %+v, want it on two rows", m)
	}
	if n := len(term.Find("error", true)); n != 2 {
		t.Errorf("case-sensitive Find found %d, want 2", n)
	}
	if n := len(term.Find("nothing like it", false)); n != 0 {
		t.Errorf("Find of absent text found %d", n)
	}
	if term.Find("", false) != nil {
		t.Error("Find of nothing found something")
	}

	// Showing a match far up scrolls to it and selects it.
	term.ShowMatch(got[0])
	tt.Frame()
	if s := selectionText(term); s != "ERROR" {
		t.Errorf("ShowMatch selected %q, want ERROR", s)
	}
	term.mu.Lock()
	sb := term.term.Scrollbar()
	term.mu.Unlock()
	if row := uint64(got[0].Row); row < sb.Offset || row >= sb.Offset+sb.Len {
		t.Errorf("row %d does not show: viewport %d+%d", row, sb.Offset, sb.Len)
	}
	save(t, tt, "found")
	// Wide characters take two columns each, and the selection covers the
	// match all the same.
	term.ShowMatch(got[1])
	if s := selectionText(term); s != "error" {
		t.Errorf("ShowMatch after 日本語 selected %q", s)
	}
	term.ShowMatch(got[2])
	if s := selectionText(term); s != "error" {
		t.Errorf("ShowMatch of the wrapped match selected %q", s)
	}
	term.ClearMatch()
	if s := selectionText(term); s != "" {
		t.Errorf("ClearMatch left %q selected", s)
	}
	term.mu.Lock()
	bottom := term.term.AtBottom()
	term.mu.Unlock()
	if !bottom {
		t.Error("ClearMatch did not go back to the screen")
	}
}

// Find in a long scrollback takes a moment, not seconds.
func TestFindSpeed(t *testing.T) {
	loadLib(t)
	if testing.Short() {
		t.Skip("slow")
	}
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.Resize(120, 40)
	var b strings.Builder
	for i := range 100000 {
		b.WriteString("2026-11-06 12:00:00 INFO request handled in 12ms id=" + strconv.Itoa(i) + "\r\n")
	}
	term.Feed([]byte(b.String()))
	start := time.Now()
	n := len(term.Find("id=99999", false))
	took := time.Since(start)
	term.mu.Lock()
	rows := term.term.Scrollbar().Total
	term.mu.Unlock()
	t.Logf("Find over %d rows took %v, %d matches", rows, took, n)
	if n != 1 {
		t.Errorf("found %d, want 1", n)
	}
	if took > 100*time.Millisecond {
		t.Errorf("Find took %v", took)
	}
}
