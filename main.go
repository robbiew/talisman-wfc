package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hpcloud/tail"
	"github.com/rivo/tview"
	"gopkg.in/ini.v1"
)

type NodeStatus struct {
	User     string
	Location string
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

	// Get the log path from the [paths] section
	logPath := cfg.Section("paths").Key("log path").String()
	if logPath == "" {
		log.Fatalf("Log path not found in talisman.ini")
	}

	// Construct the full log file path
	logFilePath := filepath.Join(*talismanPath, logPath, "talisman.log")

	// Check if the log file exists
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		log.Fatalf("Log file not found at: %s", logFilePath)
	}

	app := tview.NewApplication()
	table := tview.NewTable().SetBorders(true)

	// Initialize the table headers
	table.SetCell(0, 0, tview.NewTableCell("Node").SetSelectable(false))
	table.SetCell(0, 1, tview.NewTableCell("User").SetSelectable(false))
	table.SetCell(0, 2, tview.NewTableCell("Location").SetSelectable(false))

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
				table.SetCell(0, 0, tview.NewTableCell("Node").SetSelectable(false))
				table.SetCell(0, 1, tview.NewTableCell("User").SetSelectable(false))
				table.SetCell(0, 2, tview.NewTableCell("Location").SetSelectable(false))

				row := 1
				for node, status := range nodeStatus {
					table.SetCell(row, 0, tview.NewTableCell(node))
					table.SetCell(row, 1, tview.NewTableCell(status.User))
					table.SetCell(row, 2, tview.NewTableCell(status.Location))
					row++
				}
			})
		}
	}()

	if err := app.SetRoot(table, true).Run(); err != nil {
		log.Fatalf("Error running application: %v", err)
	}
}
