package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/term"
)

var errInterrupted = errors.New("interrupted")

// runeDisplayWidth returns the terminal display width of a rune.
// CJK characters occupy 2 cells; most others occupy 1.
func runeDisplayWidth(r rune) int {
	if r < 32 || r == 0x7F {
		return 0
	}
	if unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Hangul, r) ||
		(r >= 0xFF01 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6) {
		return 2
	}
	return 1
}

// sliceDisplayWidth returns the total display width of a rune slice.
func sliceDisplayWidth(rs []rune) int {
	w := 0
	for _, r := range rs {
		w += runeDisplayWidth(r)
	}
	return w
}

// readLine reads a line of input with cursor movement and proper Unicode/IME support.
// The prompt is displayed on stderr. Returns the entered text.
func readLine(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())

	if !term.IsTerminal(fd) {
		return readLineFallback(prompt)
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return readLineFallback(prompt)
	}
	defer term.Restore(fd, oldState)

	fmt.Fprint(os.Stderr, prompt)

	var line []rune
	pos := 0

	refresh := func() {
		fmt.Fprint(os.Stderr, "\r")
		fmt.Fprint(os.Stderr, prompt)
		fmt.Fprint(os.Stderr, string(line))
		fmt.Fprint(os.Stderr, "\033[K")
		if pos < len(line) {
			w := sliceDisplayWidth(line[pos:])
			if w > 0 {
				fmt.Fprintf(os.Stderr, "\033[%dD", w)
			}
		}
	}

	readByte := func() (byte, error) {
		var b [1]byte
		_, err := os.Stdin.Read(b[:])
		return b[0], err
	}

	readFullRune := func(first byte) (rune, error) {
		if first < 0x80 {
			return rune(first), nil
		}
		var size int
		switch {
		case first&0xE0 == 0xC0:
			size = 2
		case first&0xF0 == 0xE0:
			size = 3
		case first&0xF8 == 0xF0:
			size = 4
		default:
			return utf8.RuneError, nil
		}
		data := make([]byte, size)
		data[0] = first
		for i := 1; i < size; i++ {
			b, err := readByte()
			if err != nil {
				return utf8.RuneError, err
			}
			data[i] = b
		}
		r, _ := utf8.DecodeRune(data)
		return r, nil
	}

	insertRune := func(r rune) {
		line = append(line, 0)
		copy(line[pos+1:], line[pos:])
		line[pos] = r
		pos++
		refresh()
	}

	for {
		b, err := readByte()
		if err != nil {
			return string(line), err
		}

		switch {
		case b == 13: // Enter
			fmt.Fprint(os.Stderr, "\r\n")
			return string(line), nil

		case b == 3: // Ctrl+C
			fmt.Fprint(os.Stderr, "\r\n")
			return "", errInterrupted

		case b == 4: // Ctrl+D
			if len(line) == 0 {
				fmt.Fprint(os.Stderr, "\r\n")
				return "", errInterrupted
			}

		case b == 127 || b == 8: // Backspace
			if pos > 0 {
				pos--
				copy(line[pos:], line[pos+1:])
				line = line[:len(line)-1]
				refresh()
			}

		case b == 1: // Ctrl+A — beginning of line
			pos = 0
			refresh()

		case b == 5: // Ctrl+E — end of line
			pos = len(line)
			refresh()

		case b == 21: // Ctrl+U — clear line
			line = line[:0]
			pos = 0
			refresh()

		case b == 11: // Ctrl+K — kill to end
			line = line[:pos]
			refresh()

		case b == 23: // Ctrl+W — delete word backward
			if pos > 0 {
				newPos := pos
				for newPos > 0 && line[newPos-1] == ' ' {
					newPos--
				}
				for newPos > 0 && line[newPos-1] != ' ' {
					newPos--
				}
				copy(line[newPos:], line[pos:])
				line = line[:len(line)-(pos-newPos)]
				pos = newPos
				refresh()
			}

		case b == 27: // ESC — escape sequence
			b2, err := readByte()
			if err != nil {
				continue
			}
			if b2 != '[' {
				continue
			}
			b3, err := readByte()
			if err != nil {
				continue
			}
			switch b3 {
			case 'C': // Right arrow
				if pos < len(line) {
					pos++
					refresh()
				}
			case 'D': // Left arrow
				if pos > 0 {
					pos--
					refresh()
				}
			case 'H': // Home
				pos = 0
				refresh()
			case 'F': // End
				pos = len(line)
				refresh()
			case '3': // Delete key (ESC [ 3 ~)
				b4, _ := readByte()
				if b4 == '~' && pos < len(line) {
					copy(line[pos:], line[pos+1:])
					line = line[:len(line)-1]
					refresh()
				}
			}

		case b >= 0x80: // UTF-8 multi-byte start
			r, err := readFullRune(b)
			if err != nil {
				continue
			}
			if r != utf8.RuneError {
				insertRune(r)
			}

		case b >= 32: // Printable ASCII
			insertRune(rune(b))
		}
	}
}

// readLineFallback uses bufio for non-TTY input.
func readLineFallback(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
