//go:build mage

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

const (
	binary = "bin/sgtm"
	plist  = "cmd/sgtm/Info.plist"
)

// Build compiles the macOS Bluetooth binary with privacy metadata embedded.
func Build() error {
	if err := os.MkdirAll("bin", 0o755); err != nil {
		return err
	}
	ldflags := fmt.Sprintf(`-linkmode external -extldflags "-Wl,-sectcreate,__TEXT,__info_plist,%s"`, plist)
	if err := sh.RunV("go", "build", "-ldflags", ldflags, "-o", binary, "./cmd/sgtm"); err != nil {
		return err
	}
	return sh.RunV("codesign", "--force", "--sign", "-", "--identifier", "com.nzoschke.sgtm", binary)
}

// Sqlc regenerates typed SQLite access code from db/schema.sql and db/query.sql.
func Sqlc() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(wd)
	if err := os.Chdir("db"); err != nil {
		return err
	}
	return sh.RunV("sqlc", "generate")
}

// Scan builds and scans nearby BLE advertisements. Override flags with ARGS='--duration 30s'.
func Scan() error {
	mg.Deps(Build)
	return sh.RunV(binary, sgtmArgs("scan", "--duration", "20s")...)
}

// Inspect builds and inspects a BLE device. Pass flags with ARGS='--name decibel'.
func Inspect() error {
	mg.Deps(Build)
	return sh.RunV(binary, sgtmArgs("inspect")...)
}

// Dashboard builds and runs the local dashboard. Pass flags with ARGS='--listen :8081'.
func Dashboard() error {
	mg.Deps(Build)
	return sh.RunV(binary, sgtmArgs("dashboard")...)
}

// Clean removes generated build output.
func Clean() error {
	return os.RemoveAll("bin")
}

func sgtmArgs(command string, defaults ...string) []string {
	args := []string{command}
	if env := strings.TrimSpace(os.Getenv("ARGS")); env != "" {
		return append(args, splitArgs(env)...)
	}
	return append(args, defaults...)
}

func splitArgs(input string) []string {
	var (
		args       []string
		current    strings.Builder
		quote      rune
		escaped    bool
		inArgument bool
	)
	for _, r := range input {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
			inArgument = true
		case r == '\\':
			escaped = true
			inArgument = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
			inArgument = true
		case r == ' ' || r == '\t' || r == '\n':
			if inArgument {
				args = append(args, current.String())
				current.Reset()
				inArgument = false
			}
		default:
			current.WriteRune(r)
			inArgument = true
		}
	}
	if escaped {
		current.WriteRune('\\')
	}
	if inArgument {
		args = append(args, current.String())
	}
	return args
}
