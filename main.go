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
}

func main() {
	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel, syscall.SIGINT, syscall.SIGTERM)
	cmdChannel := make(chan Command, 10)
	outputChannel := make(chan string, 100)

	state := &AppState{
		cooldown:    time.Minute,
		containerID: "my_container",
	}

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
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			processInput(scanner.Text(), cmdChannel, outputChannel)
		}
	}()

	for {
		select {
		case <-sigChannel:
			outputChannel <- "\nShutting Down"
			return

		case cmd := <-cmdChannel:
			state.Lock()
			switch cmd.Action {
			case "redeploy":
				outputChannel <- fmt.Sprintf("Redeploying container: %s", state.containerID)
				cancelTimer(state.activeTimer)
				redeploy(state.containerID, outputChannel)

			case "pause":
				state.paused = true
				cancelTimer(state.activeTimer)
				outputChannel <- fmt.Sprintf("Auto-redeploying Paused for Container: %s", state.containerID)

			case "resume":
				state.paused = false
				outputChannel <- fmt.Sprintf("Auto-redeploying Enabled for Container: %s", state.containerID)

			case "cooldown":
				if duration, err := time.ParseDuration(cmd.Payload); err == nil {
					state.cooldown = duration
					outputChannel <- fmt.Sprintf("Cooldown set to %v", duration)
				} else {
					outputChannel <- fmt.Sprintf("Invalid duration: %v", err)
				}

			case "help":
				printHelp(outputChannel)

			default:
				outputChannel <- fmt.Sprintf("Unknown Command: %s", cmd.Action)
			}

			state.Unlock()
			outputChannel <- "PROMPT"
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
    help - This help menu
	`

	outputChannel <- help
}

func redeploy(containerID string, outputChannel chan<- string) {
	outputChannel <- fmt.Sprintf("Restarting container %s", containerID)
}

func cancelTimer(timer *time.Timer) {
	if timer != nil {
		timer.Stop()
	}
}