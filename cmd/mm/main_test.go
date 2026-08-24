package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFindProjectSearchesParents(t *testing.T) {
	root := t.TempDir()
	writeTestMakefile(t, root, "menu: ## Pick a target\n\t@true\n")
	nested := filepath.Join(root, "one", "two")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := findProject(nested)
	if err != nil {
		t.Fatalf("findProject() error = %v", err)
	}
	if got != root {
		t.Fatalf("findProject() = %q, want %q", got, root)
	}
}

func TestFindProjectChoosesNearestMatchingMenu(t *testing.T) {
	root := t.TempDir()
	writeTestMakefile(t, root, "menu:\n\t@true\n")
	child := filepath.Join(root, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestMakefile(t, child, "build:\n\t@true\n")
	nested := filepath.Join(child, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := findProject(nested)
	if err != nil {
		t.Fatalf("findProject() error = %v", err)
	}
	if got != root {
		t.Fatalf("findProject() = %q, want parent menu %q", got, root)
	}

	writeTestMakefile(t, child, "menu: build ## Child picker\n\t@true\n")
	got, err = findProject(nested)
	if err != nil {
		t.Fatalf("findProject() after child menu error = %v", err)
	}
	if got != child {
		t.Fatalf("findProject() = %q, want nearest menu %q", got, child)
	}
}

func TestFindProjectUsesMakeDefaultFileOrder(t *testing.T) {
	root := t.TempDir()
	writeTestMakefile(t, root, "menu:\n\t@true\n")
	if err := os.WriteFile(filepath.Join(root, "GNUmakefile"), []byte("build:\n\t@true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := findProject(root)
	if err == nil {
		t.Fatal("findProject() error = nil, want GNUmakefile without menu to mask Makefile")
	}
}

func TestFindProjectIgnoresMenuMentionsOutsideTargets(t *testing.T) {
	root := t.TempDir()
	writeTestMakefile(t, root, "# menu: comment only\nOTHER := menu:\nbuild:\n\t@printf 'menu: recipe text\\n'\n")

	_, err := findProject(root)
	if err == nil {
		t.Fatal("findProject() error = nil, want no menu target")
	}
}

func TestRunMenuForwardsStreamsAndExitCode(t *testing.T) {
	root := t.TempDir()
	writeTestMakefile(t, root, "menu:\n\t@true\n")
	stdin := strings.NewReader("input")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	called := false

	code := runMenu(root, stdin, stdout, stderr, func(project string, gotIn io.Reader, gotOut, gotErr io.Writer) int {
		called = true
		if project != root {
			t.Errorf("project = %q, want %q", project, root)
		}
		if gotIn != stdin || gotOut != stdout || gotErr != stderr {
			t.Error("runMenu() did not forward terminal streams")
		}
		return 23
	})

	if !called {
		t.Fatal("make runner was not called")
	}
	if code != 23 {
		t.Fatalf("runMenu() = %d, want 23", code)
	}
}

func TestRunMenuReportsMissingProject(t *testing.T) {
	stderr := &bytes.Buffer{}
	code := runMenu(t.TempDir(), strings.NewReader(""), io.Discard, stderr, func(string, io.Reader, io.Writer, io.Writer) int {
		t.Fatal("make runner called without a project menu")
		return 0
	})

	if code == 0 {
		t.Fatal("runMenu() = 0, want nonzero")
	}
	if !strings.Contains(stderr.String(), "no Makefile with a menu target") {
		t.Fatalf("stderr = %q, want missing-menu diagnostic", stderr.String())
	}
}

func TestMakeCommandUsesNearestProject(t *testing.T) {
	project := filepath.Join("tmp", "project")
	cmd := makeCommand(project)
	want := []string{"make", "--no-print-directory", "-C", project, "menu"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("makeCommand().Args = %q, want %q", cmd.Args, want)
	}
}

func TestInstallMMConfiguresUnixShellsIdempotently(t *testing.T) {
	home := t.TempDir()
	configHome := filepath.Join(home, "config")
	source := filepath.Join(home, "source-mm")
	if err := os.WriteFile(source, []byte("host-native-mm"), 0o755); err != nil {
		t.Fatal(err)
	}
	bashrc := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(bashrc, []byte("# existing shell setup\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	cfg := installConfig{
		goos:       "linux",
		home:       home,
		configHome: configHome,
		executable: source,
	}
	first, err := installMM(cfg)
	if err != nil {
		t.Fatalf("installMM() error = %v", err)
	}
	wantPath := filepath.Join(home, ".local", "bin", "mm")
	if first.path != wantPath {
		t.Fatalf("installed path = %q, want %q", first.path, wantPath)
	}
	if !first.binaryChanged || first.profileChanges != 5 {
		t.Fatalf("first install changes = %+v, want binary and five profiles", first)
	}

	installed, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != "host-native-mm" {
		t.Fatalf("installed binary = %q", installed)
	}
	info, err := os.Stat(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed mode = %o, want executable", info.Mode().Perm())
	}

	profiles := map[string]string{
		bashrc:                        "export PATH=",
		filepath.Join(home, ".zshrc"): "export PATH=",
		filepath.Join(configHome, "fish", "conf.d", "mm.fish"): "fish_add_path",
		filepath.Join(configHome, "powershell", "profile.ps1"): "$env:PATH",
		filepath.Join(configHome, "nushell", "env.nu"):         "$env.PATH",
	}
	before := make(map[string][]byte, len(profiles))
	for path, wantSnippet := range profiles {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !bytes.Contains(content, []byte(wantSnippet)) {
			t.Errorf("%s does not contain %q:\n%s", path, wantSnippet, content)
		}
		if got := bytes.Count(content, []byte(profileMarkerStart)); got != 1 {
			t.Errorf("%s marker count = %d, want 1", path, got)
		}
		before[path] = content
	}
	bashContent, err := os.ReadFile(bashrc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(bashContent, []byte("# existing shell setup\n")) {
		t.Fatalf(".bashrc existing content was replaced:\n%s", bashContent)
	}
	bashInfo, err := os.Stat(bashrc)
	if err != nil {
		t.Fatal(err)
	}
	if bashInfo.Mode().Perm() != 0o640 {
		t.Fatalf(".bashrc mode = %o, want 640", bashInfo.Mode().Perm())
	}

	second, err := installMM(cfg)
	if err != nil {
		t.Fatalf("second installMM() error = %v", err)
	}
	if second.binaryChanged || second.profileChanges != 0 {
		t.Fatalf("second install changes = %+v, want no changes", second)
	}
	for path, want := range before {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("second install changed %s", path)
		}
	}
}

func TestInstallMMPreservesSymlinkedShellProfile(t *testing.T) {
	home := t.TempDir()
	configHome := filepath.Join(home, "config")
	source := filepath.Join(home, "source-mm")
	if err := os.WriteFile(source, []byte("host-native-mm"), 0o755); err != nil {
		t.Fatal(err)
	}
	dotfiles := filepath.Join(home, "dotfiles")
	if err := os.MkdirAll(dotfiles, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dotfiles, "bashrc")
	if err := os.WriteFile(target, []byte("# managed shell setup\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".bashrc")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	_, err := installMM(installConfig{
		goos:       "linux",
		home:       home,
		configHome: configHome,
		executable: source,
	})
	if err != nil {
		t.Fatalf("installMM() error = %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("installMM() replaced the .bashrc symlink")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, []byte(profileMarkerStart)) || !bytes.HasPrefix(content, []byte("# managed shell setup\n")) {
		t.Fatalf("symlink target was not updated safely:\n%s", content)
	}
}

func TestInstallMMRollsBackUnixFilesWhenProfileUpdateFails(t *testing.T) {
	home := t.TempDir()
	configHome := filepath.Join(home, "config")
	source := filepath.Join(home, "source-mm")
	bashrc := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(source, []byte("new-mm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bashrc, []byte("original bashrc\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	_, err := installMM(installConfig{
		goos:       "linux",
		home:       home,
		configHome: configHome,
		executable: source,
		writeFile: func(path string, data []byte, mode os.FileMode, replace bool) error {
			if strings.HasSuffix(path, "mm.fish") {
				return errors.New("injected profile failure")
			}
			return writeFileAtomic(path, data, mode, replace)
		},
	})
	if err == nil || !strings.Contains(err.Error(), "injected profile failure") {
		t.Fatalf("installMM() error = %v, want injected failure", err)
	}
	content, readErr := os.ReadFile(bashrc)
	if readErr != nil || string(content) != "original bashrc\n" {
		t.Fatalf("bashrc after rollback = %q, error %v", content, readErr)
	}
	if info, statErr := os.Stat(bashrc); statErr != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("bashrc mode after rollback = %v, error %v", info, statErr)
	}
	for _, path := range []string{
		filepath.Join(home, ".local", "bin", "mm"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(configHome, "fish", "conf.d", "mm.fish"),
	} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("rollback left %s: %v", path, statErr)
		}
	}
}

func TestInstallMMConfiguresWindowsUserPath(t *testing.T) {
	home := t.TempDir()
	localAppData := filepath.Join(home, "LocalAppData")
	source := filepath.Join(home, "source-mm.exe")
	if err := os.WriteFile(source, []byte("windows-mm"), 0o755); err != nil {
		t.Fatal(err)
	}
	type invocation struct {
		name string
		args []string
	}
	var calls []invocation

	result, err := installMM(installConfig{
		goos:         "windows",
		home:         home,
		localAppData: localAppData,
		executable:   source,
		runCommand: func(name string, args ...string) error {
			calls = append(calls, invocation{name: name, args: append([]string(nil), args...)})
			return nil
		},
	})
	if err != nil {
		t.Fatalf("installMM() error = %v", err)
	}
	wantPath := filepath.Join(localAppData, "mm", "bin", "mm.exe")
	if result.path != wantPath || !result.binaryChanged {
		t.Fatalf("install result = %+v, want changed %q", result, wantPath)
	}
	if len(calls) != 1 {
		t.Fatalf("PATH command calls = %d, want 1", len(calls))
	}
	if calls[0].name != "powershell.exe" {
		t.Fatalf("PATH command = %q, want powershell.exe", calls[0].name)
	}
	if len(calls[0].args) != 4 || calls[0].args[0] != "-NoProfile" || calls[0].args[1] != "-NonInteractive" || calls[0].args[2] != "-Command" {
		t.Fatalf("PowerShell args = %q", calls[0].args)
	}
	script := calls[0].args[3]
	for _, want := range []string{
		"GetEnvironmentVariable('Path', 'User')",
		"SetEnvironmentVariable('Path'",
		"-notcontains",
		filepath.Dir(wantPath),
	} {
		if !strings.Contains(script, want) {
			t.Errorf("PowerShell script does not contain %q:\n%s", want, script)
		}
	}
}

func TestInstallMMRollsBackWindowsBinaryWhenPathUpdateFails(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "source-mm.exe")
	if err := os.WriteFile(source, []byte("new-windows-mm"), 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, "LocalAppData", "mm", "bin", "mm.exe")
	_, err := installMM(installConfig{
		goos:         "windows",
		home:         home,
		localAppData: filepath.Join(home, "LocalAppData"),
		executable:   source,
		runCommand: func(string, ...string) error {
			return errors.New("injected PATH failure")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "injected PATH failure") {
		t.Fatalf("installMM() error = %v, want injected failure", err)
	}
	if _, statErr := os.Lstat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed Windows install left binary %s: %v", destination, statErr)
	}
}

func TestInstallMMRestoresExistingWindowsBinaryWhenPathUpdateFails(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "source-mm.exe")
	destination := filepath.Join(home, "LocalAppData", "mm", "bin", "mm.exe")
	if err := os.WriteFile(source, []byte("new-windows-mm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("previous-windows-mm"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := installMM(installConfig{
		goos:         "windows",
		home:         home,
		localAppData: filepath.Join(home, "LocalAppData"),
		executable:   source,
		runCommand: func(string, ...string) error {
			return errors.New("injected PATH failure")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "injected PATH failure") {
		t.Fatalf("installMM() error = %v, want injected failure", err)
	}
	content, readErr := os.ReadFile(destination)
	if readErr != nil || string(content) != "previous-windows-mm" {
		t.Fatalf("binary after rollback = %q, error %v", content, readErr)
	}
	info, statErr := os.Stat(destination)
	if statErr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("binary mode after rollback = %v, error %v", info, statErr)
	}
}

func TestWindowsPathScriptEscapesSingleQuotes(t *testing.T) {
	script := windowsPathScript(`C:\Users\O'Brien\bin`)
	if !strings.Contains(script, `C:\Users\O''Brien\bin`) {
		t.Fatalf("windowsPathScript() did not escape path:\n%s", script)
	}
}

func TestRunInstallReportsNewTerminalRequirement(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	installCalled := false
	code := run([]string{"--install"}, strings.NewReader(""), stdout, stderr, appDependencies{
		install: func() (installResult, error) {
			installCalled = true
			return installResult{path: "/home/user/.local/bin/mm", binaryChanged: true, profileChanges: 5, reloadNeeded: true}, nil
		},
	})

	if code != 0 || !installCalled {
		t.Fatalf("run(--install) = %d, install called = %t", code, installCalled)
	}
	for _, want := range []string{"installed", "/home/user/.local/bin/mm", "new terminal"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout does not contain %q: %q", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunWithoutArgumentsOpensNearestMenu(t *testing.T) {
	root := t.TempDir()
	writeTestMakefile(t, root, "menu:\n\t@true\n")
	called := false
	code := run(nil, strings.NewReader(""), io.Discard, io.Discard, appDependencies{
		getwd: func() (string, error) { return root, nil },
		runMake: func(project string, _ io.Reader, _, _ io.Writer) int {
			called = true
			if project != root {
				t.Errorf("project = %q, want %q", project, root)
			}
			return 17
		},
	})
	if code != 17 || !called {
		t.Fatalf("run() = %d, make called = %t", code, called)
	}
}

func TestRunRejectsUnsupportedArguments(t *testing.T) {
	stderr := &bytes.Buffer{}
	code := run([]string{"check"}, strings.NewReader(""), io.Discard, stderr, appDependencies{})
	if code != 2 {
		t.Fatalf("run(check) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: mm") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func writeTestMakefile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
