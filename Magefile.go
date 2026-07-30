//go:build mage

package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

const (
	appBundle = "bin/SGTM.app"
	binary    = "bin/sgtm"
	plist     = "cmd/sgtm/Info.plist"
)

var Default = Build

// Build compiles the macOS Bluetooth binary with privacy metadata embedded.
func Build() error {
	mg.Deps(Gsx)
	if err := os.MkdirAll("bin", 0o755); err != nil {
		return err
	}
	if err := buildBinary(binary); err != nil {
		return err
	}
	return sh.RunV("codesign", "--force", "--sign", "-", "--identifier", "com.nzoschke.sgtm", binary)
}

// App builds a signed macOS .app bundle for the native Go Bluetooth dashboard.
func App() error {
	mg.Deps(Gsx)
	if err := os.RemoveAll(appBundle); err != nil {
		return err
	}
	macosDir := filepath.Join(appBundle, "Contents", "MacOS")
	resourcesDir := filepath.Join(appBundle, "Contents", "Resources")
	for _, dir := range []string{macosDir, resourcesDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := copyFile(plist, filepath.Join(appBundle, "Contents", "Info.plist"), 0o644); err != nil {
		return err
	}
	if err := copyDir("web", filepath.Join(resourcesDir, "web")); err != nil {
		return err
	}
	appBinary := filepath.Join(macosDir, "sgtm")
	if err := buildBinary(appBinary); err != nil {
		return err
	}
	if err := sh.RunV("codesign", "--force", "--sign", "-", "--identifier", "com.nzoschke.sgtm", appBinary); err != nil {
		return err
	}
	return sh.RunV("codesign", "--force", "--deep", "--sign", "-", "--identifier", "com.nzoschke.sgtm", appBundle)
}

// Gsx regenerates type-safe HTML components from ui/*.gsx.
func Gsx() error {
	return sh.RunV("go", "run", "github.com/gsxhq/gsx/cmd/gsx", "generate", "./ui")
}

// OpenApp builds and opens the native macOS app bundle.
func OpenApp() error {
	mg.Deps(App)
	return sh.RunV("open", appBundle)
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

// Native builds and runs the native Go Bluetooth dashboard. Pass flags with ARGS='--listen :8081'.
func Native() error {
	mg.Deps(Build)
	return sh.RunV(binary, sgtmArgs("native")...)
}

// Web builds and opens the browser-only Web Bluetooth dashboard.
func Web() error {
	mg.Deps(Build)
	return sh.RunV(binary, sgtmArgs("web")...)
}

// Dashboard is an alias for Native.
func Dashboard() error {
	return Native()
}

// Chrome is an alias for Web.
func Chrome() error {
	return Web()
}

// Clean removes generated build output.
func Clean() error {
	return os.RemoveAll("bin")
}

func buildBinary(out string) error {
	ldflags := fmt.Sprintf(`-linkmode external -extldflags "-Wl,-sectcreate,__TEXT,__info_plist,%s"`, plist)
	return sh.RunV("go", "build", "-ldflags", ldflags, "-o", out, "./cmd/sgtm")
}

func copyDir(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target, 0o644)
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
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
