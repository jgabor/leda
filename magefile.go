//go:build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var buildDir = filepath.Join(".", "build")

func ldflags() string {
	v := version()
	return fmt.Sprintf("-s -w -X main.version=%s", v)
}

func version() string {
	if v := os.Getenv("VERSION"); v != "" {
		return v
	}
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return "dev"
	}
	return strings.TrimSpace(string(out))
}

// Build compiles the leda binary into ./build/.
func Build() error {
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return err
	}
	bin := filepath.Join(buildDir, "leda")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	fmt.Printf("building %s (%s)\n", bin, version())
	cmd := exec.Command("go", "build", "-ldflags", ldflags(), "-o", bin, "./cmd/leda")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Install installs the leda binary to $GOPATH/bin.
func Install() error {
	fmt.Printf("installing leda (%s)\n", version())
	cmd := exec.Command("go", "install", "-ldflags", ldflags(), "./cmd/leda")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Test runs all tests with the race detector.
func Test() error {
	cmd := exec.Command("go", "test", "./...", "-race")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Clean removes the build directory.
func Clean() error {
	fmt.Println("cleaning build/")
	return os.RemoveAll(buildDir)
}
