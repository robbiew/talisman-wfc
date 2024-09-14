package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/hpcloud/tail"
	"golang.org/x/term"
	"gopkg.in/ini.v1"
)

type NodeStatus struct {
	User     string
	Location string
}

const (
	// Column widths
	nodeColWidth     = 5
	userColWidth     = 25
	locationColWidth = 25
	systemNameWidth  = 66
	quitMessageWidth = 14
	totalTableWidth  = nodeColWidth + userColWidth + locationColWidth
	headerHeight     = 4

	// Text colors
	colorNode               = WhiteHi
	colorNodeLabel          = Cyan
	colorUser               = CyanHi
	colorUserLabel          = Cyan
	colorLocation           = CyanHi
	colorLocationLabel      = Cyan
	colorLastUserLabel      = Yellow
	colorLastUser           = YellowHi
	colorSeparator          = BlackHi
	colorSystemName         = Red
	colorQuitMessage        = Red
	colorBackgroundBar      = BgWhiteHi
	colorBackgroundBarLabel = Red
)

// Regular expressions for parsing log entries
var (
	logPattern        = regexp.MustCompile(`INFO: (.+?) (logged in|loading menu|running door|running script|listing messages|posting a message) (.+?) on node (\d+)`)
	disconnectPattern = regexp.MustCompile(`INFO: Node (\d+) logged off`)
	loginPattern      = regexp.MustCompile(`INFO: (.+?) logged in on node (\d+)`)
)

// DrawTable draws the table of nodes and user statuses
func DrawTable(nodeStatus map[string]NodeStatus, maxNodes int, talismanPath string, oldState *term.State) {
	// Restore terminal to cooked mode for drawing
	term.Restore(int(os.Stdin.Fd()), oldState)
	defer term.MakeRaw(int(os.Stdin.Fd())) // Return to raw mode after drawing

	// Clear the screen
	ClearScreen()

	// Draw header art
	DisplayAnsiFile(filepath.Join(talismanPath, "gfiles", "wfc.ans"), true)
	fmt.Print(BgBlack)

	// Move the cursor to the line after the ANSI art (2 rows tall)
	MoveCursor(1, headerHeight+1) // Move cursor to the beginning of the line after the art

	// Draw table headers with colors
	fmt.Println(Reset + colorNodeLabel + PadOrTruncate("Node", nodeColWidth) + Reset +
		colorUserLabel + PadOrTruncate("User", userColWidth) + Reset +
		colorLocationLabel + PadOrTruncate("Location", locationColWidth) + Reset)
	fmt.Println(strings.Repeat(colorSeparator+"-", totalTableWidth) + Reset)

	for i := 1; i <= maxNodes; i++ {
		nodeStr := strconv.Itoa(i)
		status, exists := nodeStatus[nodeStr]

		// Set default values
		user := "waiting for caller"
		location := "-"

		// Update values if the user is on this node
		if exists {
			user = status.User
			location = status.Location
		}

		// Apply padding before adding colors
		paddedNodeStr := PadOrTruncate(nodeStr, nodeColWidth)
		paddedUser := PadOrTruncate(user, userColWidth)
		paddedLocation := PadOrTruncate(location, locationColWidth)

		// Add colors after padding
		nodeColored := colorNode + paddedNodeStr + Reset
		userColored := colorUser + paddedUser + Reset
		locationColored := colorLocation + paddedLocation + Reset

		fmt.Println(
			nodeColored +
				userColored +
				locationColored,
		)
	}
}

// findLastLoggedOffUser scans the log file for the most recent logged-off user
func findLastLoggedOffUser(logFilePath string) string {
	file, err := os.Open(logFilePath)
	checkError(err, "Error opening log file")
	defer file.Close()

	scanner := bufio.NewScanner(file)
	disconnectPattern := regexp.MustCompile(`INFO: Node (\d+) logged off`)
	userPattern := regexp.MustCompile(`INFO: (.+?) logged in on node (\d+)`)

	lastUser := "None"
	activeUsers := make(map[string]string)

	for scanner.Scan() {
		line := scanner.Text()

		// Capture logins
		if loginMatches := userPattern.FindStringSubmatch(line); len(loginMatches) > 0 {
			node := loginMatches[2]
			user := loginMatches[1]
			activeUsers[node] = user
		}

		// Capture logouts and update the last user based on node activity
		if disconnectMatches := disconnectPattern.FindStringSubmatch(line); len(disconnectMatches) > 0 {
			node := disconnectMatches[1]
			if user, exists := activeUsers[node]; exists {
				lastUser = user
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading log file: %v", err)
	}
	return lastUser
}

func loadConfig(path string) (*ini.File, error) {
	iniFilePath := filepath.Join(path, "talisman.ini")
	cfg, err := ini.Load(iniFilePath)
	checkError(err, "failed to load ini file")
	return cfg, nil
}

// Error handling function
func checkError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %v", msg, err)
	}
}

func main() {
	// Get terminal dimensions
	CursorHide()
	h, w := GetTermSize()

	// Parse command-line argument for Talisman installation path
	talismanPath := flag.String("path", "", "Path to the Talisman BBS installation")
	flag.Parse()

	if *talismanPath == "" {
		log.Fatal("Please provide the path to the Talisman BBS installation using the --path flag.")
	}

	cfg, err := loadConfig(*talismanPath)
	checkError(err, "loading configuration")

	// Get required values from the ini file
	logPath := cfg.Section("paths").Key("log path").String()
	if logPath == "" {
		log.Fatalf("Log path not found in talisman.ini")
	}

	maxNodesStr := cfg.Section("main").Key("max nodes").String()
	maxNodes, err := strconv.Atoi(maxNodesStr)
	if err != nil {
		log.Fatalf("Invalid max nodes value in talisman.ini: %v", err)
	}

	systemName := cfg.Section("main").Key("system name").String()
	if systemName == "" {
		log.Fatal("System name not found in talisman.ini")
	}

	// Construct the full log file path
	logFilePath := filepath.Join(*talismanPath, logPath, "talisman.log")

	// Check if the log file exists
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		log.Fatalf("Log file not found at: %s", logFilePath)
	}

	// Initialize variables for node status and log tailing
	nodeStatus := make(map[string]NodeStatus)

	// Start tailing the log file
	t, err := tail.TailFile(logFilePath, tail.Config{Follow: true})
	checkError(err, "Failed to tail file")

	// Enter raw mode to take full control of the terminal
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	checkError(err, "Error entering raw mode")
	defer func() {
		checkError(term.Restore(int(os.Stdin.Fd()), oldState), "restoring terminal state")
	}()

	// Display the initial screen
	lastUser := findLastLoggedOffUser(logFilePath)
	DrawTable(nodeStatus, maxNodes, *talismanPath, oldState)

	// Continuously update the screen as new log entries are read
	go func() {

		for line := range t.Lines {
			if disconnectMatches := disconnectPattern.FindStringSubmatch(line.Text); len(disconnectMatches) > 0 {
				node := disconnectMatches[1]
				delete(nodeStatus, node)
			} else if loginMatches := loginPattern.FindStringSubmatch(line.Text); len(loginMatches) > 0 {
				node := loginMatches[2]
				user := loginMatches[1]
				nodeStatus[node] = NodeStatus{User: user, Location: "logging in"}
			} else if matches := logPattern.FindStringSubmatch(line.Text); len(matches) > 0 {
				node := matches[4]
				user := matches[1]
				location := matches[3]

				// Simplify the location
				location = strings.TrimPrefix(location, "menu ")
				location = strings.TrimPrefix(location, "menus/")
				location = strings.TrimSuffix(location, ".toml")

				nodeStatus[node] = NodeStatus{User: user, Location: location}
			}

			// Redraw table and last user
			DrawTable(nodeStatus, maxNodes, *talismanPath, oldState)
			fmt.Printf(colorLastUserLabel+"\nLast User:"+Reset+colorLastUser+" %s\n"+Reset, lastUser)

			// Move the cursor to the bottom of the screen
			MoveCursor(1, h)
			PrintSpaces(w, colorBackgroundBar)

			MoveCursor(1, h)
			fmt.Printf(colorBackgroundBar+colorBackgroundBarLabel+" System Name: %s"+Reset, systemName)
			MoveCursor(w-13, h)
			fmt.Printf(colorBackgroundBar + colorBackgroundBarLabel + "Q/ESC to Quit" + Reset)
		}
	}()

	// Handle user input
	HandleKeyPress()
}
