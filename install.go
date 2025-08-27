package main

import (
	"fmt"
	"log"
	"os"
	"os/user"
)

// installService creates and installs the systemd service file for Clara.
func installService() error {
	log.Println("Installing Clara as a systemd service...")

	// Get the path of the current executable.
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not get executable path: %w", err)
	}

	// Get the current user to run the service as.
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("could not get current user: %w", err)
	}
	username := currentUser.Username

	// Use the executable's directory as the working directory.
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not get working directory: %w", err)
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=Clara Declarative Reverse Proxy
After=network.target

[Service]
Type=simple
User=%s
Group=%s
WorkingDirectory=%s
ExecStart=%s
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
`, username, username, workingDir, execPath)

	servicePath := "/etc/systemd/system/clara.service"
	log.Printf("Creating service file at %s", servicePath)
	err = os.WriteFile(servicePath, []byte(serviceContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	log.Println("Service file created successfully.")
	log.Println("Please run the following commands to enable and start the service:")
	log.Println("  sudo systemctl daemon-reload")
	log.Println("  sudo systemctl enable clara")
	log.Println("  sudo systemctl start clara")

	return nil
}
