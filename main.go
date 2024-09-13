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
	"gopkg.in/ini.v1"
)

type NodeStatus struct {
	User     string
	Location string
}

const (
	nodeColWidth     = 5
	userColWidth     = 25
	locationColWidth = 25
	systemNameWidth  = 66
	quitMessageWidth = 14
	totalTableWidth  = nodeColWidth + userColWidth + locationColWidth
)

// ClearScreen clears the terminal screen
func ClearScreen() {
	fmt.Print("\033[H\033[2J") // ANSI escape to clear screen and move to top
}

// MoveCursor moves the cursor to a specific position on the screen
func MoveCursor(x, y int) {
	fmt.Printf("\033[%d;%dH", y, x) // ANSI escape to move the cursor
}

// padOrTruncate ensures text is exactly width characters long
func padOrTruncate(text string, width int) string {
	if len(text) > width {
		return text[:width]
	}
	return text + strings.Repeat(" ", width-len(text))
}

// DrawTable draws the table of nodes and user statuses
func DrawTable(nodeStatus map[string]NodeStatus, maxNodes int) {
	ClearScreen()
	fmt.Println(padOrTruncate("Node", nodeColWidth) + padOrTruncate("User", userColWidth) + padOrTruncate("Location", locationColWidth))
	fmt.Println(strings.Repeat("-", totalTableWidth))

	for i := 1; i <= maxNodes; i++ {
		nodeStr := strconv.Itoa(i)
		status, exists := nodeStatus[nodeStr]

		// If no user is on this node, display "waiting for caller"
		user := "waiting for caller"
		location := "-"
		if exists {
			user = status.User
			location = status.Location
		}

		fmt.Println(padOrTruncate(nodeStr, nodeColWidth) + padOrTruncate(user, userColWidth) + padOrTruncate(location, locationColWidth))
	}
}

// findLastLoggedOffUser scans the log file for the most recent logged-off user
func findLastLoggedOffUser(logFilePath string) string {
	file, err := os.Open(logFilePath)
	if err != nil {
		log.Printf("Error opening log file: %v", err)
		return "None"
	}
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

func main() {
	// Parse command-line argument for Talisman installation path
	talismanPath := flag.String("path", "", "Path to the Talisman BBS installation")
	flag.Parse()

	if *talismanPath == "" {
		log.Fatal("Please provide the path to the Talisman BBS installation using the --path flag.")
	}

	// Find and parse talisman.ini file
	iniFilePath := filepath.Join(*talismanPath, "talisman.ini")
	cfg, err := ini.Load(iniFilePath)
	if err != nil {
		log.Fatalf("Failed to load ini file: %v", err)
	}

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
	if err != nil {
		log.Fatalf("Failed to tail file: %v", err)
	}

	// Display the initial screen
	lastUser := findLastLoggedOffUser(logFilePath)
	ClearScreen()
	DrawTable(nodeStatus, maxNodes)
	fmt.Printf("\nLast User: %s\n", lastUser)
	fmt.Printf("\nSystem Name: %s | Hit Q to quit\n", systemName)

	// Continuously update the screen as new log entries are read
	go func() {
		logPattern := regexp.MustCompile(`INFO: (.+?) (logged in|loading menu|running door|running script|listing messages|posting a message) (.+?) on node (\d+)`)
		disconnectPattern := regexp.MustCompile(`INFO: Node (\d+) logged off`)
		loginPattern := regexp.MustCompile(`INFO: (.+?) logged in on node (\d+)`)

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
			DrawTable(nodeStatus, maxNodes)
			fmt.Printf("\nLast User: %s\n", lastUser)
			fmt.Printf("\nSystem Name: %s | Hit Q to quit\n", systemName)
		}
	}()

	// Handle user input for quitting
	for {
		var input string
		fmt.Scanln(&input)
		if strings.ToLower(input) == "q" {
			break
		}
	}
}
