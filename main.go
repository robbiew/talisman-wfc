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
	"time"

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
	userColWidth     = 20
	locationColWidth = 20
	systemNameWidth  = 66
	quitMessageWidth = 14
	totalTableWidth  = nodeColWidth + userColWidth + locationColWidth
	headerHeight     = 4

	// Maximum number of lines to read from the log file (whole file is loaded on startup)
	maxLogLines = 200

	// Text colors
	colorNode               = WhiteHi
	colorNodeLabel          = Cyan
	colorUser               = CyanHi
	colorUserLabelUnet      = Green
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

var (
	// Regular expressions for parsing log entries
	logPattern        = regexp.MustCompile(`INFO: (.+?) (logged in|loading menu|running door|running script|listing messages|posting a message) (.+?) on node (\d+)`)
	disconnectPattern = regexp.MustCompile(`INFO: Node (\d+) logged off`)
	loginPattern      = regexp.MustCompile(`INFO: (.+?) logged in on node (\d+)`)
	connectionPattern = regexp.MustCompile(`INFO: Connection From: (.+?) on Node (\d+)`)
	menuPattern       = regexp.MustCompile(`INFO: (.+?) loading menu (.+?) on node (\d+)`)
	newUserPattern    = regexp.MustCompile(`INFO: New user signing up on node (\d+)`)

	// Change "sysop" to the actual username you want to exclude
	excludeUser = "j0hnny a1pha"
)

// Function to count today's calls excluding the specified user
func countTodaysCalls(logFilePath string) int {
	// Get the current date in YYYY-MM-DD format
	today := time.Now().Format("2006-01-02")
	count := 0

	// Open the log file
	file, err := os.Open(logFilePath)
	if err != nil {
		log.Printf("Error opening log file: %v", err)
		return count
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Check if the log line is from today
		if strings.HasPrefix(line, today) {
			// Check for a login match
			if loginMatches := loginPattern.FindStringSubmatch(line); len(loginMatches) > 0 {
				user := loginMatches[1]
				// Increment the count if the user is not the one to exclude
				if user != excludeUser {
					count++
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading log file: %v", err)
	}

	return count
}

func formatCell(text string, width int, color string) string {
	return Reset + color + PadOrTruncate(text, width) + Reset
}

// DrawTableRow draws a single row in the table.
func DrawTableRow(nodeNum int, status NodeStatus, maxNodes int, talismanPath string) {
	// Calculate the row position based on nodeNum
	row := headerHeight + 2 + nodeNum

	// Move cursor to the specific row and column
	MoveCursor(1, row)

	// Determine color based on user status
	userColor := colorUser
	if status.User == "waiting for caller" {
		userColor = colorUserLabelUnet // Default color for "waiting for caller"
	}

	// Format and print the node data
	nodeStr := strconv.Itoa(nodeNum)
	fmt.Println(
		" " +
			formatCell(nodeStr, nodeColWidth, colorNode) +
			formatCell(status.User, userColWidth, userColor) +
			formatCell(status.Location, locationColWidth, colorLocation),
	)
}

// DrawTable draws the full table of nodes and user statuses.
func DrawTable(nodeStatus map[string]NodeStatus, maxNodes int, talismanPath string, oldState *term.State) {
	// Restore terminal to cooked mode for drawing
	term.Restore(int(os.Stdin.Fd()), oldState)
	defer term.MakeRaw(int(os.Stdin.Fd())) // Return to raw mode after drawing

	// Clear the screen
	ClearScreen()

	// Draw header art
	DisplayAnsiFile(filepath.Join(talismanPath, "gfiles", "wfc.ans"), true)
	fmt.Print(BgBlack)

	// Move the cursor to the line after the ANSI art (2 rows tall), offset by 1 column
	MoveCursor(1, headerHeight+1) // Move cursor to column 2 instead of 1

	// Draw table headers with colors
	fmt.Println(
		" " +
			formatCell("Node", nodeColWidth, colorNodeLabel) +
			formatCell("User", userColWidth, colorUserLabel) +
			formatCell("Location", locationColWidth, colorLocationLabel),
	)
	fmt.Println(" " + strings.Repeat(colorSeparator+"-", totalTableWidth) + Reset)

	// Draw all rows initially
	for i := 1; i <= maxNodes; i++ {
		status, exists := nodeStatus[strconv.Itoa(i)]

		if !exists {
			status = NodeStatus{User: "waiting for caller", Location: "-"}
		}

		DrawTableRow(i, status, maxNodes, talismanPath)
	}
}

func findLastLoggedOffUser(logFilePath string, numLines int) string {
	// Use tail to read the entire file
	t, err := tail.TailFile(logFilePath, tail.Config{
		Follow:   false,
		Location: &tail.SeekInfo{Offset: 0, Whence: os.SEEK_SET},
	})
	if err != nil {
		log.Printf("Error tailing log file: %v", err)
		return "None"
	}
	defer t.Cleanup()

	var lines []string
	for line := range t.Lines {
		lines = append(lines, line.Text)
	}

	// Process only the last `numLines` lines
	lastUser := "None"
	activeUsers := make(map[string]string)
	for _, line := range lines[max(0, len(lines)-numLines):] {
		// Capture logins
		if loginMatches := loginPattern.FindStringSubmatch(line); len(loginMatches) > 0 {
			node := loginMatches[2]
			user := loginMatches[1]
			activeUsers[node] = user
		}
		// Capture logouts
		if disconnectMatches := disconnectPattern.FindStringSubmatch(line); len(disconnectMatches) > 0 {
			node := disconnectMatches[1]
			if user, exists := activeUsers[node]; exists {
				lastUser = user
			}
		}
	}
	return lastUser
}

func loadConfig(path string) (*ini.File, error) {
	iniFilePath := filepath.Join(path, "talisman.ini")
	cfg, err := ini.Load(iniFilePath)
	if err != nil {
		log.Fatalf("Failed to load configuration file at %s: %v", iniFilePath, err)
	}
	return cfg, nil
}

// Error handling function
func checkError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %v", msg, err)
	}
}

// Draws the footer with system name and quit instruction
func drawFooter(h, w int, systemName string) {
	MoveCursor(1, h)
	PrintSpaces(w, colorBackgroundBar)

	MoveCursor(1, h)
	fmt.Printf(colorBackgroundBar+colorBackgroundBarLabel+" System Name: %s"+Reset, systemName)
	MoveCursor(w-13, h)
	fmt.Printf(colorBackgroundBar + colorBackgroundBarLabel + "Q/ESC to Quit" + Reset)
}

func main() {
	// Hide the cursor
	CursorHide()

	// Get terminal dimensions
	h, w, err := GetTermSize()
	if err != nil {
		log.Printf("Error getting terminal size, using default: %v", err)
		h, w = 25, 80 // default size
	}

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
		log.Fatalf("Log path not found in talisman.ini. Please check the configuration.")
	}

	maxNodesStr := cfg.Section("main").Key("max nodes").String()
	maxNodes, err := strconv.Atoi(maxNodesStr)
	if err != nil {
		log.Fatalf("Invalid max nodes value in talisman.ini: %v. Please provide a valid integer.", err)
	}

	systemName := cfg.Section("main").Key("system name").String()
	if systemName == "" {
		log.Fatal("System name not found in talisman.ini. Please provide a system name.")
	}

	// Construct the full log file path
	logFilePath := filepath.Join(*talismanPath, logPath, "talisman.log")

	// Check if the log file exists
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		log.Printf("Log file not found at: %s. Starting with an empty log.", logFilePath)
		file, err := os.Create(logFilePath)
		checkError(err, fmt.Sprintf("Failed to create log file at %s", logFilePath))
		file.Close()
	}

	// Initialize variables for node status, active users and log tailing
	nodeStatus := make(map[string]NodeStatus, maxNodes)
	activeUsers := make(map[string]string) // node number to username mapping

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
	lastUser := findLastLoggedOffUser(logFilePath, maxLogLines) // Read last 100 lines to get recent entries
	DrawTable(nodeStatus, maxNodes, *talismanPath, oldState)

	// Count today's calls
	todaysCalls := countTodaysCalls(logFilePath)

	// Draw the initial footer
	drawFooter(h, w, systemName)

	// Print the last user and today's calls
	MoveCursor(1, h-3)
	fmt.Printf(colorLastUserLabel+" Last User:"+Reset+colorLastUser+" %s\n"+Reset, lastUser)
	MoveCursor(1, h-2)
	fmt.Printf(colorLastUserLabel+" Today's Calls: "+Reset+colorLastUser+"%d (excluding %s)\n"+Reset, todaysCalls, excludeUser)

	// Create a ticker to limit the redraw frequency
	ticker := time.NewTicker(500 * time.Millisecond) // Redraw every 500ms
	defer ticker.Stop()

	// Continuously update the screen as new log entries are read
	go func() {
		for {
			select {
			case line := <-t.Lines:
				updatedNodes := make(map[int]NodeStatus) // Track updated nodes

				if connectionMatches := connectionPattern.FindStringSubmatch(line.Text); len(connectionMatches) > 0 {
					ip := connectionMatches[1]
					node := connectionMatches[2]
					// Update NodeStatus with "Unknown User" in User column and IP in Location column
					nodeStatus[node] = NodeStatus{User: "Unknown User", Location: ip}
					nodeNum, _ := strconv.Atoi(node)
					updatedNodes[nodeNum] = nodeStatus[node]
				} else if loginMatches := loginPattern.FindStringSubmatch(line.Text); len(loginMatches) > 0 {
					node := loginMatches[2]
					user := loginMatches[1]
					// Set the user and display "logging in..." in the Location column
					nodeStatus[node] = NodeStatus{User: user, Location: "logging in..."}
					nodeNum, _ := strconv.Atoi(node)
					updatedNodes[nodeNum] = nodeStatus[node]

					// Track the logged-in user
					activeUsers[node] = user

					// Recount today's calls
					todaysCalls = countTodaysCalls(logFilePath)
				} else if newUserMatches := newUserPattern.FindStringSubmatch(line.Text); len(newUserMatches) > 0 {
					node := newUserMatches[1]
					nodeNum, _ := strconv.Atoi(node)
					// Update NodeStatus with "New User" and "Signing up..." information
					nodeStatus[node] = NodeStatus{User: "New User", Location: "Signing up..."}
					updatedNodes[nodeNum] = nodeStatus[node]

					// Track the new user as a placeholder until they log in
					// Do not add "New User" to activeUsers since it's not the actual username
				} else if menuMatches := menuPattern.FindStringSubmatch(line.Text); len(menuMatches) > 0 {
					user := menuMatches[1]
					menuName := strings.Title(strings.TrimSuffix(filepath.Base(menuMatches[2]), ".toml")) // Capitalize the menu name
					node := menuMatches[3]
					nodeStatus[node] = NodeStatus{User: user, Location: "At " + menuName + " Menu"}
					nodeNum, _ := strconv.Atoi(node)
					updatedNodes[nodeNum] = nodeStatus[node]
				} else if matches := logPattern.FindStringSubmatch(line.Text); len(matches) > 0 {
					node := matches[4]
					user := matches[1]
					location := matches[3]

					// Simplify the location and handle specific cases
					location = strings.TrimPrefix(location, "menu ")
					location = strings.TrimPrefix(location, "menus/")
					location = strings.TrimSuffix(location, ".toml")
					location = "At " + strings.Title(location)

					nodeStatus[node] = NodeStatus{User: user, Location: location}
					nodeNum, _ := strconv.Atoi(node)
					updatedNodes[nodeNum] = nodeStatus[node]
				} else if disconnectMatches := disconnectPattern.FindStringSubmatch(line.Text); len(disconnectMatches) > 0 {
					node := disconnectMatches[1]
					// Ensure we only update lastUser if there was an actual user logged in
					if user, exists := activeUsers[node]; exists && user != "New User" {
						lastUser = user // Update the last user to the one who logged off
					}
					delete(activeUsers, node) // Remove the user from the active users
					delete(nodeStatus, node)
					nodeNum, _ := strconv.Atoi(node)
					updatedNodes[nodeNum] = NodeStatus{User: "waiting for caller", Location: "-"}

					// Recount today's calls
					todaysCalls = countTodaysCalls(logFilePath)
				}

				// Only redraw if there are changes
				select {
				case <-ticker.C:
					// Redraw only on ticker or when there's a change
					for nodeNum, status := range updatedNodes {
						DrawTableRow(nodeNum, status, maxNodes, *talismanPath)
					}

					// Update the last user display and today's calls
					MoveCursor(1, h-3)
					fmt.Print("\033[K") // Clear the line
					fmt.Printf(colorLastUserLabel+" Last User:"+Reset+colorLastUser+" %s\n"+Reset, lastUser)
					MoveCursor(1, h-2)
					fmt.Print("\033[K") // Clear the line
					fmt.Printf(colorLastUserLabel+" Today's Calls: "+Reset+colorLastUser+"%d (excluding %s)\n"+Reset, todaysCalls, excludeUser)

					// Move the cursor to the bottom of the screen
					drawFooter(h, w, systemName)
				default:
					// Skip redraw if not needed
				}
			}
		}
	}()

	// Handle user input
	HandleKeyPress()
}
