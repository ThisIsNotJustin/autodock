# Auto Dock

Auto Dock is a Go based CLI tool for monitoring changes in a specified director and redeploying a container when changes are detected. It includes features such as user-defined folder watching, adjustable cooldown periods, and the ability to pause or resume auto-redeploy functionality.

## Features

- **Dynamic Folder Watching**: Add or remove directories to monitor at runtime.

- **Auto-Redeploy**: Automatically redeploys the specified container when file changes are detected.

- **Cooldown Period**: Configurable cooldown period to avoid redundant redeploys.

- **Pause/Resume**: Temporarily stop and resume auto-redeploy functionality.

- **Status Reporting**: View the current application state, including watched folders and cooldown settings.

## Requirements

- [Go](https://golang.org/dl/)

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/ThisIsNotJustin/autodock
   ```
3. Run the program:
   ```bash
   go run main.go
   ```

---

## Usage

When the program starts, it initializes with default settings. You can interact with the program using the following CLI commands:

### Commands

- Add a folder to watch for file changes.
  ```bash
  > watch ./myfolder
  ```
- Stop watching a folder.
  ```bash
  > watchRemove ./myfolder
  ```
- Display a list of all currently watched folders.
  ```bash
  > watchList
  ```
- Reset watched folders list.
  ```bash
  > reset
  ```
- Immediately redeploy the container.
  ```bash
  > redeploy
  ```
- Pause the auto-redeploy feature.
  ```bash
  > pause
  ```
- Resume the auto-redeploy feature.
  ```bash
  > resume
  ```
- Set the cooldown period (e.g., `30s`, `1m`, `2h`).
  ```bash
  > cooldown 30s
  ```
- Display the current state of the application.
  ```bash
  > status
  ```
- Show a list of available commands.
  ```bash
  > help
  ```

---

## Example Workflow

1. Start the program.
2. Add folders to monitor:
   ```bash
   > watch ./projects
   > watch ./config
   ```
3. Change a file in one of the watched folders. The program will:
   - Detect the change.
   - Wait for the configured cooldown period.
   - Redeploy the container.
4. View the application status:
   ```bash
   > status
   --- Current Status ---
   Container ID: my_container
   Auto-Redeployment: true
   Last Change: 2025-01-26 14:31:17
   Cooldown: 30s
   Watched Folders:
    - ./projects
    - ./config
   ```

## Example Pt2
```bash
go run main.go
> help
  Available commands:
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

> status

--- Current Status ---

Container ID: my_container
Auto-Redeployment: true
Last Change: 0001-01-01 00:00:00 +0000 UTC
Cooldown: 1m0s
> watchList
--- Watched Folders ---
> cooldown 30s
[!] Cooldown set to 30s
> watch ./testFolder
Watching folder: ./testFolder
> watchList
--- Watched Folders ---
./testFolder
> Change in testFolder/text.txt
[!] Change detected, starting redeployment
[!] Redeploying Container
Restarting container my_container
> watch ./testFolder2
Watching folder: ./testFolder2
> watchList
--- Watched Folders ---
./testFolder
./testFolder2
> Change in testFolder2/text2.txt
[!] Change detected, starting redeployment
[!] Redeploying Container
Restarting container my_container
> watchList
--- Watched Folders ---
./testFolder
./testFolder2
> watchRemove ./testFolder2
Removed ./testFolder2 from watch list
> watchList
--- Watched Folders ---
./testFolder
> status

--- Current Status ---

Container ID: my_container
Auto-Redeployment: true
Last Change: 2025-01-27 22:47:14.986287 -0600 CST m=+124.997843793
Cooldown: 30s
> reset
Successfully reset watched folders
> watchList
--- Watched Folders ---
> 
```

---

## Acknowledgments

- Inspired by the need for efficient container management during development cycles.
- Utilizes the `fsnotify` library for file system event detection.

---

## Troubleshooting

- **Watcher Error**: Ensure that the folder paths provided are valid and accessible.
- **Cooldown Misconfiguration**: Use valid time formats (e.g., `30s`, `1m`).
- **Redeployment Not Triggering**: Ensure auto-redeploy is not paused and cooldown is correctly configured.

---