package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func nativeCmd(args []string) error {
	filtered := args[:0]
	foreground := false
	for _, arg := range args {
		if arg == "--foreground" {
			foreground = true
			continue
		}
		filtered = append(filtered, arg)
	}
	if foreground || runningInAppBundle() {
		return dashboardCmd(filtered)
	}
	return launchNativeTerminal(filtered)
}

func runningInAppBundle() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.Contains(filepath.ToSlash(exe), ".app/Contents/MacOS/")
}

func launchNativeTerminal(args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.Abs(exe); err != nil {
		return err
	}
	if err := os.MkdirAll(".context", 0o755); err != nil {
		return err
	}

	command := []string{shellQuote(exe), "dashboard"}
	for _, arg := range args {
		command = append(command, shellQuote(arg))
	}
	script := "#!/bin/zsh\ncd " + shellQuote(wd) + " || exit 1\nexec " + strings.Join(command, " ") + "\n"
	path := filepath.Join(".context", "run-native-dashboard.command")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		return err
	}
	if err := exec.Command("open", "-a", "Terminal", path).Start(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Native dashboard launched in Terminal.app\n")
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
