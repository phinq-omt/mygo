package terminal

import (
	"strings"

	"github.com/egoist/mygo/plugins/terminal/internal/vt"
)

// CommandOutput returns what a command the shell ran printed: n 0 is the
// last command's, 1 the one's before it, and so on, from the screen and the
// scrollback, with soft-wrapped lines joined and trailing blanks trimmed. A
// command still running counts as the last, with what it printed so far.
// It reports false when the shell marks no prompts with OSC 133 (shell
// integration), on the alternate screen, and when the scrollback no longer
// holds the prompt the command was typed at.
func (t *Terminal) CommandOutput(n int) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil || t.term.AltScreen() || n < 0 {
		return "", false
	}
	term := t.term
	_, cy := term.Cursor()
	y := term.ScrollbackRows() + cy
	prompt := func(y int) vt.RowPrompt {
		p, _ := term.ScreenRowPrompt(y)
		return p
	}
	// skipPrompt moves y up over the prompt it is on, from its last row to
	// above its first, where 133;A started it.
	skipPrompt := func() {
		for y >= 0 && prompt(y) == vt.PromptContinuation {
			y--
		}
		if y >= 0 && prompt(y) == vt.PromptStart {
			y--
		}
	}
	// The prompt the cursor is at, when no command runs.
	skipPrompt()
	for {
		end := y
		for y >= 0 && prompt(y) == vt.NoPrompt {
			y--
		}
		if y < 0 {
			return "", false
		}
		if n == 0 {
			return screenText(term, y+1, end), true
		}
		n--
		skipPrompt()
	}
}

// screenText returns the text of rows first to last of the screen and its
// scrollback, a row that soft-wraps joined to the next, trailing blanks
// trimmed. t.mu is held.
func screenText(term *vt.Terminal, first, last int) string {
	var b strings.Builder
	for y := first; y <= last; y++ {
		text, _ := term.ScreenRowText(y)
		_, wrap := term.ScreenRowPrompt(y)
		if wrap && y < last {
			b.WriteString(string(text))
			continue
		}
		b.WriteString(strings.TrimRight(string(text), " "))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), " \n")
}
