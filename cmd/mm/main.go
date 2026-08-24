package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

var makefileNames = [...]string{"GNUmakefile", "makefile", "Makefile"}

type makeRunner func(project string, stdin io.Reader, stdout, stderr io.Writer) int

type appDependencies struct {
	getwd   func() (string, error)
	install func() (installResult, error)
	runMake makeRunner
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, appDependencies{}))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, deps appDependencies) int {
	if len(args) == 1 && args[0] == "--install" {
		install := deps.install
		if install == nil {
			install = installCurrent
		}
		result, err := install()
		if err != nil {
			fmt.Fprintf(stderr, "mm: install: %v\n", err)
			return 1
		}
		if result.binaryChanged || result.profileChanges > 0 {
			fmt.Fprintf(stdout, "mm installed at %s\n", result.path)
		} else {
			fmt.Fprintf(stdout, "mm is already installed at %s\n", result.path)
		}
		if result.reloadNeeded {
			fmt.Fprintln(stdout, "Open a new terminal before running mm.")
		} else {
			fmt.Fprintln(stdout, "mm is ready in this shell.")
		}
		return 0
	}
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: mm")
		return 2
	}

	getwd := deps.getwd
	if getwd == nil {
		getwd = os.Getwd
	}
	cwd, err := getwd()
	if err != nil {
		fmt.Fprintf(stderr, "mm: get current directory: %v\n", err)
		return 1
	}
	runMake := deps.runMake
	if runMake == nil {
		runMake = executeMake
	}
	return runMenu(cwd, stdin, stdout, stderr, runMake)
}

func runMenu(start string, stdin io.Reader, stdout, stderr io.Writer, run makeRunner) int {
	project, err := findProject(start)
	if err != nil {
		fmt.Fprintf(stderr, "mm: %v\n", err)
		return 1
	}
	return run(project, stdin, stdout, stderr)
}

func findProject(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve current directory: %w", err)
	}

	for {
		makefile, found, err := defaultMakefile(dir)
		if err != nil {
			return "", err
		}
		if found {
			hasMenu, err := makefileDefinesMenu(makefile)
			if err != nil {
				return "", err
			}
			if hasMenu {
				return dir, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("no Makefile with a menu target found from %s", start)
}

func defaultMakefile(dir string) (string, bool, error) {
	for _, name := range makefileNames {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		switch {
		case err == nil && info.Mode().IsRegular():
			return path, true, nil
		case err == nil:
			return "", false, fmt.Errorf("%s is not a regular file", path)
		case errors.Is(err, os.ErrNotExist):
			continue
		default:
			return "", false, fmt.Errorf("inspect %s: %w", path, err)
		}
	}
	return "", false, nil
}

func makefileDefinesMenu(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] == '\t' {
			continue
		}
		if comment := bytes.IndexByte(line, '#'); comment >= 0 {
			line = line[:comment]
		}
		colon := bytes.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		if assignment := bytes.IndexByte(line, '='); assignment >= 0 && assignment < colon {
			continue
		}
		for _, target := range bytes.Fields(line[:colon]) {
			if bytes.Equal(target, []byte("menu")) {
				return true, nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	return false, nil
}

func makeCommand(project string) *exec.Cmd {
	return exec.Command("make", "--no-print-directory", "-C", project, "menu")
}

func executeMake(project string, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd := makeCommand(project)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(stderr, "mm: run make: %v\n", err)
		return 1
	}
	return 0
}
