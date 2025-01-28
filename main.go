package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Command struct {
	Action  string
	Payload string
}

type AppState struct {
	sync.Mutex
	cooldown    time.Duration
	containerID string
	activeTimer *time.Timer
	paused      bool
	lastChange  time.Time
	watchDirs   map[string]bool
}

func main() {
	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel, syscall.SIGINT, syscall.SIGTERM)
	cmdChannel := make(chan Command, 10)
	outputChannel := make(chan string, 100)
	eventChannel := make(chan fsnotify.Event, 100)

	state := &AppState{
		cooldown:    time.Minute,
		containerID: "my_container",
		paused:      false,
		watchDirs:   make(map[string]bool),
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		outputChannel <- fmt.Sprintf("Error creating watcher: %v", err)
		return
	}
	defer watcher.Close()

	go func() {
		for msg := range outputChannel {
			if msg == "PROMPT" {
				fmt.Print("> ")
			} else {
				fmt.Println(msg)
			}
		}
	}()

	outputChannel <- "PROMPT"

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if event.Has(fsnotify.Write) {
					eventChannel <- event
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}

				outputChannel <- fmt.Sprintf("Watcher Error: %v", err)
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			input := strings.TrimSpace(scanner.Text())
			if input != "" {
				processInput(input, cmdChannel, outputChannel)
			}
		}
	}()

	for {
		select {
		case <-sigChannel:
			outputChannel <- "\nShutting Down"
			close(outputChannel)
			return

		case cmd := <-cmdChannel:
			handleCommand(cmd, state, watcher, outputChannel)

		case event := <-eventChannel:
			state.Lock()

			if !state.paused {
				fileChange(state, event, outputChannel)
			}

			state.Unlock()
		}
	}
}

func processInput(input string, cmdChannel chan<- Command, outputChannel chan<- string) {
	input = strings.TrimSpace(input)
	if input == "" {
		outputChannel <- "PROMPT"
		return
	}

	slices := strings.SplitN(input, " ", 2)
	cmd := Command{Action: slices[0]}
	if len(slices) > 1 {
		cmd.Payload = slices[1]
	}

	select {
	case cmdChannel <- cmd:
	default:
		outputChannel <- "Command queue full"
		outputChannel <- "PROMPT"
	}
}

func printHelp(outputChannel chan<- string) {
	help := `  Available commands:
    redeploy - Immediate deployment
    pause - Pause auto-redeploy
    resume - Resume auto-redeploy
    cooldown (seconds) - Set auto-redeploy cooldown 'cooldown 60s'
    status - See current status
    watch (folder path) - Add folder to watch list
    watchRemove (folder path) - Remove folder from watch list
    watchList - Display list of folders watching for changes
    reset - Remove all folders from watch list
    help - This help menu
	`

	outputChannel <- help
}

func printStatus(state *AppState, outputChannel chan<- string) {
	outputChannel <- fmt.Sprintln("\n--- Current Status ---")
	outputChannel <- fmt.Sprintf("Container ID: %s", state.containerID)
	outputChannel <- fmt.Sprintf("Auto-Redeployment: %t", !state.paused)
	outputChannel <- fmt.Sprintf("Last Change: %v", state.lastChange)
	outputChannel <- fmt.Sprintf("Cooldown: %v", state.cooldown)
}

func redeploy(containerID string, outputChannel chan<- string) {
	outputChannel <- fmt.Sprintf("Restarting container %s", containerID)
}

func cancelTimer(timer *time.Timer) {
	if timer != nil {
		timer.Stop()
	}
}

func fileChange(state *AppState, event fsnotify.Event, outputChannel chan<- string) {
	current := time.Now()
	if state.activeTimer != nil {
		return
	}

	outputChannel <- fmt.Sprintf("Change in %s", event.Name)

	if current.Sub(state.lastChange) > state.cooldown {
		outputChannel <- "[!] Change detected, starting redeployment"
		state.activeTimer = time.AfterFunc(state.cooldown, func() {
			state.Lock()
			defer state.Unlock()

			if !state.paused {
				outputChannel <- "[!] Redeploying Container"
				redeploy(state.containerID, outputChannel)
				state.lastChange = time.Now()
			}

			state.activeTimer = nil
			outputChannel <- "PROMPT"
		})
	} else {
		outputChannel <- "change within cooldown, don't care"
		outputChannel <- "PROMPT"
	}
}

func handleCommand(cmd Command, state *AppState, watcher *fsnotify.Watcher, outputChannel chan<- string) {
	state.Lock()
	switch cmd.Action {
	case "redeploy":
		outputChannel <- fmt.Sprintf("[!] Redeploying container: %s", state.containerID)
		cancelTimer(state.activeTimer)
		redeploy(state.containerID, outputChannel)

	case "pause":
		state.paused = true
		cancelTimer(state.activeTimer)
		outputChannel <- fmt.Sprintf("[!] Auto-Redeployment Paused for Container: %s", state.containerID)

	case "resume":
		state.paused = false
		outputChannel <- fmt.Sprintf("[!] Auto-Redeployment Enabled for Container: %s", state.containerID)

	case "cooldown":
		if duration, err := time.ParseDuration(cmd.Payload); err == nil {
			state.cooldown = duration
			outputChannel <- fmt.Sprintf("[!] Cooldown set to %v", duration)
		} else {
			outputChannel <- fmt.Sprintf("Invalid duration: %v", err)
		}

	case "status":
		printStatus(state, outputChannel)

	case "watch":
		if cmd.Payload == "" {
			outputChannel <- "Please specify a folder ie 'watch ./testFolder1'"
		} else {
			err := watcher.Add(cmd.Payload)
			if err != nil {
				outputChannel <- fmt.Sprintf("Error adding directory: %v", err)
				return
			}

			state.watchDirs[cmd.Payload] = true
			outputChannel <- fmt.Sprintf("Watching folder: %s", cmd.Payload)
		}

	case "watchRemove":
		if cmd.Payload == "" {
			outputChannel <- "Please specify a folder to remove ie 'watchRemove .testFolder1'"
		} else {
			err := watcher.Remove(cmd.Payload)
			if err != nil {
				outputChannel <- fmt.Sprintf("Error removing folder: %v", err)
				return
			}

			delete(state.watchDirs, cmd.Payload)
			outputChannel <- fmt.Sprintf("Removed %s from watch list", cmd.Payload)
		}

	case "watchList":
		outputChannel <- "--- Watched Folders ---"
		for dir := range state.watchDirs {
			outputChannel <- dir
		}

	case "reset":
		for dir := range state.watchDirs {
			watcher.Remove(dir)
			delete(state.watchDirs, dir)
		}

		outputChannel <- "Successfully reset watched folders"

	case "help":
		printHelp(outputChannel)

	default:
		outputChannel <- fmt.Sprintf("Unknown Command: %s", cmd.Action)
	}

	state.Unlock()
	outputChannel <- "PROMPT"
}
