package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
	"golang.org/x/text/encoding/charmap"
)

const (
	// Common ANSI escape sequences
	Esc = "\u001B["
	Osc = "\u001B]"
	Bel = "\u0007"

	CursorBackward = Esc + "D"
	CursorPrevLine = Esc + "F"
	CursorLeft     = Esc + "G"
	CursorTop      = Esc + "d"
	CursorTopLeft  = Esc + "H"

	CursorBlinkEnable  = Esc + "?12h"
	CursorBlinkDisable = Esc + "?12I"

	ScrollUp   = Esc + "S"
	ScrollDown = Esc + "T"

	TextInsertChar = Esc + "@"
	TextDeleteChar = Esc + "P"
	TextEraseChar  = Esc + "X"
	TextInsertLine = Esc + "L"
	TextDeleteLine = Esc + "M"

	EraseRight  = Esc + "K"
	EraseLeft   = Esc + "1K"
	EraseLine   = Esc + "2K"
	EraseDown   = Esc + "J"
	EraseUp     = Esc + "1J"
	EraseScreen = Esc + "2J"

	Black     = Esc + "30m"
	Red       = Esc + "31m"
	Green     = Esc + "32m"
	Yellow    = Esc + "33m"
	Blue      = Esc + "34m"
	Magenta   = Esc + "35m"
	Cyan      = Esc + "36m"
	White     = Esc + "37m"
	BlackHi   = Esc + "30;1m"
	RedHi     = Esc + "31;1m"
	GreenHi   = Esc + "32;1m"
	YellowHi  = Esc + "33;1m"
	BlueHi    = Esc + "34;1m"
	MagentaHi = Esc + "35;1m"
	CyanHi    = Esc + "36;1m"
	WhiteHi   = Esc + "37;1m"

	BgBlack     = Esc + "40m"
	BgRed       = Esc + "41m"
	BgGreen     = Esc + "42m"
	BgYellow    = Esc + "43m"
	BgBlue      = Esc + "44m"
	BgMagenta   = Esc + "45m"
	BgCyan      = Esc + "46m"
	BgWhite     = Esc + "47m"
	BgBlackHi   = Esc + "40;1m"
	BgRedHi     = Esc + "41;1m"
	BgGreenHi   = Esc + "42;1m"
	BgYellowHi  = Esc + "43;1m"
	BgBlueHi    = Esc + "44;1m"
	BgMagentaHi = Esc + "45;1m"
	BgCyanHi    = Esc + "46;1m"
	BgWhiteHi   = Esc + "47;1m"

	Reset = Esc + "0m"
)

// HandleKeyPress handles detecting key press events
func HandleKeyPress() {
	for {
		b := make([]byte, 1)
		_, err := os.Stdin.Read(b)
		if err != nil {
			log.Printf("Error reading input: %v", err)
			continue
		}
		// Exit if 'q', 'Q', or 'Esc' is pressed
		if b[0] == 'q' || b[0] == 'Q' || b[0] == 27 { // 27 is the ASCII code for the Escape key
			CursorShow()
			break
		}
	}
}

// PrintSpaces prints a number of spaces equal to the terminal width with a given background color.
func PrintSpaces(width int, bgColor string) {
	spaces := strings.Repeat(" ", width) // Create a string with the number of spaces equal to the width
	fmt.Print(bgColor + spaces + Reset)  // Print the spaces with the background color and reset at the end
}

// PadOrTruncate handles padding or truncating the string while ignoring ANSI color codes
func PadOrTruncate(text string, width int) string {
	plainText := StripAnsi(text)
	textLen := len(plainText)
	if textLen == width {
		return text
	} else if textLen > width {
		return text[:width]
	}
	return text + strings.Repeat(" ", width-textLen)
}

// StripAnsi removes ANSI color codes from a string
func StripAnsi(str string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(str, "")
}

// ClearScreen clears the terminal screen
func ClearScreen() {
	fmt.Print("\033[H\033[2J") // ANSI escape to clear screen and move to top
}

// MoveCursor moves the cursor to a specific position on the screen
func MoveCursor(x, y int) {
	fmt.Printf("\033[%d;%dH", y, x) // ANSI escape to move the cursor
}

// Move the cursor n cells to up.
func CursorUp(n int) {
	fmt.Printf(Esc+"%dA", n)
}

// Move the cursor n cells to down.
func CursorDown(n int) {
	fmt.Printf(Esc+"%dB", n)
}

// Move the cursor n cells to right.
func CursorForward(n int) {
	fmt.Printf(Esc+"%dC", n)
}

// Move the cursor n cells to left.
func CursorBack(n int) {
	fmt.Printf(Esc+"%dD", n)
}

// Move cursor to beginning of the line n lines down.
func CursorNextLine(n int) {
	fmt.Printf(Esc+"%dE", n)
}

// Move cursor to beginning of the line n lines up.
func CursorPreviousLine(n int) {
	fmt.Printf(Esc+"%dF", n)
}

// Move cursor horizontally to x.
func CursorHorizontalAbsolute(x int) {
	fmt.Printf(Esc+"%dG", x)
}

// Show the cursor.
func CursorShow() {
	fmt.Print(Esc + "?25h")
}

// Hide the cursor.
func CursorHide() {
	fmt.Print(Esc + "?25l")
}

// Save the screen.
func SaveScreen() {
	fmt.Print(Esc + "?47h")
}

// Restore the saved screen.
func RestoreScreen() {
	fmt.Print(Esc + "?47l")
}

func GetTermSize() (int, int, error) {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get terminal size: %w", err)
	}
	return height, width, nil
}

func DisplayAnsiFile(filePath string, localDisplay bool) {
	content, err := ReadAnsiFile(filePath)
	if err != nil {
		log.Fatalf("Error reading file %s: %v", filePath, err)
	}
	PrintAnsi(content, 0, localDisplay)
}

func ReadAnsiFile(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// Print ANSI art with a delay between lines
func PrintAnsi(artContent string, delay int, localDisplay bool) { // localDisplay as an argument for UTF-8 conversion
	noSauce := TrimStringFromSauce(artContent) // strip off the SAUCE metadata
	lines := strings.Split(noSauce, "\r\n")

	for i, line := range lines {
		if localDisplay {
			// Convert line from CP437 to UTF-8
			utf8Line, err := charmap.CodePage437.NewDecoder().String(line)
			if err != nil {
				fmt.Printf("Error converting to UTF-8: %v\n", err)
				continue
			}
			line = utf8Line
		}

		if i < len(lines)-1 && i != 24 { // Check for the 25th line (index 24)
			fmt.Println(line) // Print with a newline
		} else {
			fmt.Print(line) // Print without a newline (for the 25th line and the last line of the art)
		}
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}
}

func TrimStringFromSauce(s string) string {
	if idx := strings.Index(s, "COMNT"); idx != -1 {
		string := s
		delimiter := "COMNT"
		leftOfDelimiter := strings.Split(string, delimiter)[0]
		trim := TrimLastChar(leftOfDelimiter)
		return trim
	}
	if idx := strings.Index(s, "SAUCE00"); idx != -1 {
		string := s
		delimiter := "SAUCE00"
		leftOfDelimiter := strings.Split(string, delimiter)[0]
		trim := TrimLastChar(leftOfDelimiter)
		return trim
	}
	return s
}

func TrimLastChar(s string) string {
	r, size := utf8.DecodeLastRuneInString(s)
	if r == utf8.RuneError && (size == 0 || size == 1) {
		size = 0
	}
	return s[:len(s)-size]
}

func PrintAnsiLoc(artfile string, x int, y int) {
	yLoc := y

	noSauce := TrimStringFromSauce(artfile) // strip off the SAUCE metadata
	s := bufio.NewScanner(strings.NewReader(string(noSauce)))

	for s.Scan() {
		fmt.Fprintf(os.Stdout, Esc+strconv.Itoa(yLoc)+";"+strconv.Itoa(x)+"f"+s.Text())
		yLoc++
	}
}

// Print text at an X, Y location
func PrintStringLoc(text string, x int, y int) {
	fmt.Fprintf(os.Stdout, Esc+strconv.Itoa(y)+";"+strconv.Itoa(x)+"f"+text)

}

// Horizontally center some text.
func CenterText(s string, w int) {
	fmt.Fprintf(os.Stdout, (fmt.Sprintf("%[1]*s", -w, fmt.Sprintf("%[1]*s", (w+len(s))/2, s))))
}
