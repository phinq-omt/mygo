package terminal

import (
	"unicode"

	"github.com/egoist/mygo/plugins/terminal/internal/vt"
)

// Match is text that Find found, from row Row to row EndRow, the same row
// unless the match runs on across a line the terminal wrapped. Rows count
// from the oldest the scrollback keeps, so a match moves when output
// pushes rows out of it: find again once the terminal has printed.
type Match struct {
	Row, EndRow int
	// start and end are the indexes of the first and the last rune of the
	// match in the text of their rows, which ShowMatch makes columns.
	start, end int
}

// MaxMatches is the most matches Find returns.
const MaxMatches = 10000

// runePos is a rune of a line: its row, and its index in the row's text.
type runePos struct{ row, i int }

// Find returns where query appears in the screen and its scrollback, the
// oldest first, at most MaxMatches. A match may run on across a line that
// the terminal wrapped. With caseSensitive false, letters match whatever
// their case.
func (t *Terminal) Find(query string, caseSensitive bool) []Match {
	needle := []rune(query)
	if len(needle) == 0 {
		return nil
	}
	if !caseSensitive {
		fold(needle)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return nil
	}
	var (
		matches []Match
		line    []rune
		at      []runePos
	)
	search := func() {
		if !caseSensitive {
			fold(line)
		}
		for i := 0; i+len(needle) <= len(line) && len(matches) < MaxMatches; i++ {
			if equal(line[i:i+len(needle)], needle) {
				s, e := at[i], at[i+len(needle)-1]
				matches = append(matches, Match{s.row, e.row, s.i, e.i})
				i += len(needle) - 1
			}
		}
		line, at = line[:0], at[:0]
	}
	// The text of every row in one call, as reading cells one by one
	// takes a second over a long scrollback.
	rows := t.term.Rows()
	for y, text := range rows {
		i := 0
		for _, r := range text {
			line = append(line, r)
			at = append(at, runePos{y, i})
			i++
		}
		// An empty row has no text to wrap: spare asking.
		if text == "" || !t.term.RowWraps(y) {
			search()
			if len(matches) >= MaxMatches {
				break
			}
		}
	}
	search()
	return matches
}

// ShowMatch scrolls the scrollback to m, unless it shows already, and
// selects it, so that it stands out and copies.
func (t *Terminal) ShowMatch(m Match) {
	t.mu.Lock()
	if t.term != nil {
		sb := t.term.Scrollbar()
		top, n := int(sb.Offset), int(sb.Len)
		if m.Row < top || m.EndRow >= top+n {
			t.term.ScrollToRow(max(0, (m.Row+m.EndRow)/2-n/2))
		}
		start, ok1 := t.term.ScreenCellAt(t.column(m.Row, m.start), m.Row)
		end, ok2 := t.term.ScreenCellAt(t.column(m.EndRow, m.end), m.EndRow)
		if ok1 && ok2 {
			s := vt.NewSelection(start, end)
			t.term.SetSelection(&s)
		}
	}
	t.mu.Unlock()
	t.redraw()
}

// column returns the column of rune i of row y's text; t.mu is held.
func (t *Terminal) column(y, i int) int {
	_, cols := t.term.ScreenRowText(y)
	if len(cols) == 0 {
		return 0
	}
	return cols[max(0, min(i, len(cols)-1))]
}

// ClearMatch ends what ShowMatch did: nothing is selected, and the
// terminal shows its screen again.
func (t *Terminal) ClearMatch() {
	t.mu.Lock()
	if t.term != nil {
		t.term.SetSelection(nil)
		t.term.ScrollToBottom()
	}
	t.mu.Unlock()
	t.redraw()
}

// fold lowers the letters of s in place, keeping its length.
func fold(s []rune) {
	for i, r := range s {
		s[i] = unicode.ToLower(r)
	}
}

func equal(a, b []rune) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SelectedText returns the text selected in the terminal, "" when nothing
// is, for a search to start from.
func (t *Terminal) SelectedText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return ""
	}
	s, _ := t.term.SelectionText()
	return s
}
