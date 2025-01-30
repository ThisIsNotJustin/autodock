package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
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
	inputPaused bool
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

	if err := exec.Command("docker", "--version").Run(); err != nil {
		outputChannel <- "Install Docker to use this program!"
		os.Exit(1)
	}
	outputChannel <- "\n         Auto Docker"
	outputChannel <- "-----------------------------"
	outputChannel <- "Automated deployment system with file watching"
	outputChannel <- "Type 'help' for list of available commands"
	outputChannel <- "Press 'Ctrl + C' to escape this CLI tool or Docker terminal\n"

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
		reader := bufio.NewReader(os.Stdin)
		for {
			state.Lock()
			paused := state.inputPaused
			state.Unlock()

			if paused {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			os.Stdin.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			input, _, _ := reader.ReadLine()
			if len(input) > 0 {
				processInput(string(input), cmdChannel, outputChannel)
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
	outputChannel <- fmt.Sprintln("--- Current Status ---")
	outputChannel <- fmt.Sprintf("Container ID: %s", state.containerID)
	outputChannel <- fmt.Sprintf("Auto-Redeployment: %t", !state.paused)
	outputChannel <- fmt.Sprintf("Last Change: %v", state.lastChange)
	outputChannel <- fmt.Sprintf("Cooldown: %v\n", state.cooldown)
}

func redeploy(state *AppState, outputChannel chan<- string) {
	go func() {
		cwd, err := os.Getwd()
		if err != nil {
			outputChannel <- fmt.Sprintf("Error getting current directory: %v", err)
			outputChannel <- "PROMPT"
			return
		}

		state.Lock()
		state.inputPaused = true
		state.Unlock()

		defer func() {
			state.Lock()
			state.inputPaused = false
			state.Unlock()
		}()

		// docker in current terminal
		outputChannel <- "Beginning Redeploy"
		cmd := exec.Command("docker", "compose", "down")
		output, err := cmd.CombinedOutput()
		if err != nil {
			outputChannel <- fmt.Sprintf("Error: %v\nOutput: %s", err, string(output))
			outputChannel <- "PROMPT"
			return
		}
		outputChannel <- string(output)

		// OS-specific terminal commands
		var newcmd *exec.Cmd
		switch runtime.GOOS {
		case "linux": // linux
			newcmd = exec.Command(
				"xterm",
				"-hold",
				"-e",
				fmt.Sprintf("cd %s && sudo docker compose up --build", cwd),
			)
		case "darwin": // macOS
			newcmd = exec.Command(
				"osascript",
				"-e",
				fmt.Sprintf(`tell application "Terminal" to do script "cd %s && sudo docker compose up --build"`, cwd),
			)
		default:
			outputChannel <- "Unsupported OS for terminal separation"
			outputChannel <- "PROMPT"
			return
		}

		err = newcmd.Start()
		if err != nil {
			outputChannel <- fmt.Sprintf("Error starting terminal: %v", err)
			outputChannel <- "PROMPT"
			return
		}

		outputChannel <- "Redeploy running in new terminal window"
		outputChannel <- "PROMPT"
	}()
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
				redeploy(state, outputChannel)
				state.lastChange = time.Now()
			}

			state.activeTimer = nil
		})
	} else {
		outputChannel <- "Change detected within cooldown, please wait\n"
		outputChannel <- "PROMPT"
	}
}

func handleCommand(cmd Command, state *AppState, watcher *fsnotify.Watcher, outputChannel chan<- string) {
	state.Lock()
	sendPrompt := true

	switch cmd.Action {
	case "redeploy":
		outputChannel <- fmt.Sprintf("[!] Redeploying container: %s", state.containerID)
		cancelTimer(state.activeTimer)
		redeploy(state, outputChannel)
		sendPrompt = false

	case "pause":
		state.paused = true
		cancelTimer(state.activeTimer)
		outputChannel <- fmt.Sprintf("[!] Auto-Redeployment Paused for Container: %s\n", state.containerID)

	case "resume":
		state.paused = false
		outputChannel <- fmt.Sprintf("[!] Auto-Redeployment Enabled for Container: %s\n", state.containerID)

	case "cooldown":
		if duration, err := time.ParseDuration(cmd.Payload); err == nil {
			state.cooldown = duration
			outputChannel <- fmt.Sprintf("[!] Cooldown set to %v\n", duration)
		} else {
			outputChannel <- fmt.Sprintf("Invalid duration: %v\n", err)
		}

	case "status":
		printStatus(state, outputChannel)

	case "watch":
		if cmd.Payload == "" {
			outputChannel <- "Please specify a folder ie 'watch ./testFolder1'\n"
		} else {
			err := watcher.Add(cmd.Payload)
			if err != nil {
				outputChannel <- fmt.Sprintf("Error adding directory: %v\n", err)
				return
			}

			state.watchDirs[cmd.Payload] = true
			outputChannel <- fmt.Sprintf("Watching folder: %s\n", cmd.Payload)
		}

	case "watchRemove":
		if cmd.Payload == "" {
			outputChannel <- "Please specify a folder to remove ie 'watchRemove .testFolder1'"
		} else {
			err := watcher.Remove(cmd.Payload)
			if err != nil {
				outputChannel <- fmt.Sprintf("Error removing folder: %v\n", err)
				return
			}

			delete(state.watchDirs, cmd.Payload)
			outputChannel <- fmt.Sprintf("Removed %s from watch list\n", cmd.Payload)
		}

	case "watchList":
		outputChannel <- "--- Watched Folders ---"
		for dir := range state.watchDirs {
			outputChannel <- dir
		}
		outputChannel <- "\n"

	case "reset":
		for dir := range state.watchDirs {
			watcher.Remove(dir)
			delete(state.watchDirs, dir)
		}

		outputChannel <- "Successfully reset watched folders\n"

	case "help":
		printHelp(outputChannel)

	default:
		outputChannel <- fmt.Sprintf("Unknown Command: %s\n", cmd.Action)
	}

	state.Unlock()

	if sendPrompt {
		outputChannel <- "PROMPT"
	}
}
