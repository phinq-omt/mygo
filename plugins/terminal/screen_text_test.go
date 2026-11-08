package terminal

import "testing"

func TestScreenText(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.Resize(40, 3)
	if got := term.ScreenText(); got != "" {
		t.Fatalf("an empty screen: %q", got)
	}
	term.Feed([]byte("one\r\ntwo\r\nthree\r\nfour   \r\n✻ Working (esc to interrupt)"))
	if got, want := term.ScreenText(), "three\nfour\n✻ Working (esc to interrupt)"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	term.Feed([]byte("\r\x1b[2K> "))
	if got, want := term.ScreenText(), "three\nfour\n>"; got != want {
		t.Fatalf("after the line was erased: %q, want %q", got, want)
	}
}
