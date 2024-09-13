package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/hpcloud/tail"
	"github.com/rivo/tview"
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
	totalTableWidth  = nodeColWidth + userColWidth + locationColWidth
)

// padOrTruncate ensures text is exactly width characters long.
func padOrTruncate(text string, width int) string {
	if len(text) > width {
		return text[:width]
	}
	return text + strings.Repeat(" ", width-len(text))
}

// mapColorName maps a color name string to tcell color.
func mapColorName(colorName string) tcell.Color {
	colorName = strings.ToLower(colorName)
	switch colorName {
	case "black":
		return tcell.ColorBlack
	case "red":
		return tcell.ColorRed
	case "green":
		return tcell.ColorGreen
	case "yellow":
		return tcell.ColorYellow
	case "blue":
		return tcell.ColorBlue
	case "magenta":
		return tcell.ColorPurple
	case "cyan":
		return tcell.ColorDarkCyan
	case "white":
		return tcell.ColorWhite
	case "bright black":
		return tcell.ColorDarkGray
	case "bright red":
		return tcell.ColorIndianRed
	case "bright green":
		return tcell.ColorLightGreen
	case "bright yellow":
		return tcell.ColorLightYellow
	case "bright blue":
		return tcell.ColorLightBlue
	case "bright magenta":
		return tcell.ColorFuchsia
	case "bright cyan":
		return tcell.ColorLightCyan
	case "bright white":
		return tcell.ColorLightGray
	default:
		return tcell.ColorWhite // Default to white if unknown
	}
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

	inputBackground := cfg.Section("main").Key("input background").String()
	inputForeground := cfg.Section("main").Key("input foreground").String()

	// Construct the full log file path
	logFilePath := filepath.Join(*talismanPath, logPath, "talisman.log")

	// Check if the log file exists
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		log.Fatalf("Log file not found at: %s", logFilePath)
	}

	app := tview.NewApplication()
	table := tview.NewTable().SetBorders(false)

	// Initialize the table headers with fixed widths
	table.SetCell(0, 0, tview.NewTableCell(padOrTruncate("Node", nodeColWidth)).SetSelectable(false))
	table.SetCell(0, 1, tview.NewTableCell(padOrTruncate("User", userColWidth)).SetSelectable(false))
	table.SetCell(0, 2, tview.NewTableCell(padOrTruncate("Location", locationColWidth)).SetSelectable(false))

	// Message bar at the bottom with system name and quit message
	messageBar := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(false).
		SetTextAlign(tview.AlignLeft).
		SetWrap(false)

	// Update the message bar to dynamically right-align "Hit Q to quit"
	messageBar.SetDynamicColors(true)
	messageBar.SetRegions(false)
	messageBar.SetText(fmt.Sprintf("%s%s",
		padOrTruncate(systemName, 40),
		fmt.Sprintf("%30s", "Hit Q to quit")))

	// Adjust the background and foreground colors based on the ini file
	messageBar.SetBackgroundColor(mapColorName(inputBackground))
	messageBar.SetTextColor(mapColorName(inputForeground))

	// Start tailing the log file
	t, err := tail.TailFile(logFilePath, tail.Config{Follow: true})
	if err != nil {
		log.Fatalf("Failed to tail file: %v", err)
	}

	nodeStatus := make(map[string]NodeStatus)
	logPattern := regexp.MustCompile(`\[(\d+)\]  INFO: (.+?) (logged in|loading menu|running door|running script|listing messages|listing fileareas|listing file conferences|posting a message) (.+?) on node (\d+)`)
	disconnectPattern := regexp.MustCompile(`\[(\d+)\]  INFO: Node (\d+) logged off`)
	loginPattern := regexp.MustCompile(`\[(\d+)\]  INFO: (.+?) logged in on node (\d+)`)

	go func() {
		for line := range t.Lines {
			if disconnectMatches := disconnectPattern.FindStringSubmatch(line.Text); len(disconnectMatches) > 0 {
				node := disconnectMatches[2]
				delete(nodeStatus, node)
			} else if loginMatches := loginPattern.FindStringSubmatch(line.Text); len(loginMatches) > 0 {
				node := loginMatches[3]
				user := loginMatches[2]
				nodeStatus[node] = NodeStatus{User: user, Location: "logging in"}
			} else if matches := logPattern.FindStringSubmatch(line.Text); len(matches) > 0 {
				node := matches[5]
				user := matches[2]
				_ = matches[3] // Ignore the action part
				location := matches[4]

				// Simplify the location
				location = strings.TrimPrefix(location, "menu ")
				location = strings.TrimPrefix(location, "menus/")
				location = strings.TrimSuffix(location, ".toml")

				nodeStatus[node] = NodeStatus{User: user, Location: location}
			}

			app.QueueUpdateDraw(func() {
				table.Clear()
				table.SetCell(0, 0, tview.NewTableCell(padOrTruncate("Node", nodeColWidth)).SetSelectable(false))
				table.SetCell(0, 1, tview.NewTableCell(padOrTruncate("User", userColWidth)).SetSelectable(false))
				table.SetCell(0, 2, tview.NewTableCell(padOrTruncate("Location", locationColWidth)).SetSelectable(false))

				// Display node information
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

					table.SetCell(i, 0, tview.NewTableCell(padOrTruncate(nodeStr, nodeColWidth)))
					table.SetCell(i, 1, tview.NewTableCell(padOrTruncate(user, userColWidth)))
					table.SetCell(i, 2, tview.NewTableCell(padOrTruncate(location, locationColWidth)))
				}
			})
		}
	}()

	// Listen for the "Q" key to quit
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' || event.Rune() == 'Q' {
			app.Stop()
			return nil
		}
		return event
	})

	// Layout: table at the top, message bar at the bottom
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(table, 0, 1, true).
		AddItem(messageBar, 1, 0, false)

	if err := app.SetRoot(layout, true).Run(); err != nil {
		log.Fatalf("Error running application: %v", err)
	}
}
