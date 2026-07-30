package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

func chromeCmd(args []string) error {
	fs := flag.NewFlagSet("chrome", flag.ExitOnError)
	listen := fs.String("listen", ":8090", "HTTP listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := webRoot()
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	url := dashboardURL(ln.Addr().String())
	fmt.Fprintf(os.Stderr, "Chrome Web Bluetooth dashboard listening on %s\n", url)
	openDashboard(url)
	return http.Serve(ln, http.FileServer(http.Dir(root)))
}

func webRoot() (string, error) {
	candidates := []string{"web"}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "..", "web"),
			filepath.Join(exeDir, "..", "Resources", "web"),
		)
	}
	for _, candidate := range candidates {
		info, err := os.Stat(filepath.Join(candidate, "index.html"))
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("cannot find web/index.html; run from the repository root")
}
