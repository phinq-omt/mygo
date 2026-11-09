package terminal

// clearInput is the key the zsh integration binds to empty the line being
// edited, in every keymap, as ReplaceInput sends it.
const clearInput = "\x1b[7733~"

// AtPrompt reports whether the shell waits at its prompt for a command, as
// it marks it with OSC 133 (shell integration): false while a command runs,
// on the alternate screen, and for a shell that marks no prompts.
func (t *Terminal) AtPrompt() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.term != nil && !t.term.AltScreen() && t.prompt.atPrompt
}

// PromptInput returns what is typed at the shell's prompt and where the
// cursor is in it, in characters, as the zsh integration tells them on each
// change. ok is false when the shell is not at its prompt, and when it does
// not tell its input: another shell, one started before the integration
// told it, or an input too long to tell.
func (t *Terminal) PromptInput() (text string, cursor int, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil || t.term.AltScreen() || !t.prompt.atPrompt || !t.prompt.known {
		return "", 0, false
	}
	return t.prompt.input, t.prompt.cursor, true
}

// ReplaceInput puts line at the shell's prompt in place of what is typed
// there, without running it: the integration's key empties the line, and
// line is pasted, bracketed when the shell asks, so that a newline in it
// does not run it either. A shell that does not tell its input gets the
// emacs keys to go to the end of the line and kill back to its start.
func (t *Terminal) ReplaceInput(line string) {
	t.mu.Lock()
	reports := t.prompt.reports
	t.mu.Unlock()
	if reports {
		t.Send([]byte(clearInput))
	} else {
		t.Send([]byte("\x05\x15"))
	}
	t.Paste(line)
}
