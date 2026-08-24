package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	profileMarkerStart = "# >>> mm shortcut >>>"
	profileMarkerEnd   = "# <<< mm shortcut <<<"
)

type installConfig struct {
	goos         string
	home         string
	configHome   string
	localAppData string
	executable   string
	currentPath  string
	runCommand   func(name string, args ...string) error
	writeFile    func(path string, data []byte, mode os.FileMode, replace bool) error
}

type installResult struct {
	path           string
	binaryChanged  bool
	profileChanges int
	reloadNeeded   bool
}

type profileSpec struct {
	path string
	body string
}

type fileSnapshot struct {
	path   string
	data   []byte
	mode   os.FileMode
	exists bool
}

type profilePlan struct {
	snapshot fileSnapshot
	updated  []byte
	changed  bool
}

func installCurrent() (installResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return installResult{}, fmt.Errorf("resolve home directory: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return installResult{}, fmt.Errorf("resolve mm executable: %w", err)
	}
	return installMM(installConfig{
		goos:         runtime.GOOS,
		home:         home,
		configHome:   os.Getenv("XDG_CONFIG_HOME"),
		localAppData: os.Getenv("LOCALAPPDATA"),
		executable:   executable,
		currentPath:  os.Getenv("PATH"),
	})
}

func installMM(cfg installConfig) (installResult, error) {
	if cfg.home == "" {
		return installResult{}, errors.New("resolve home directory: empty path")
	}
	if cfg.executable == "" {
		return installResult{}, errors.New("resolve mm executable: empty path")
	}

	if cfg.goos == "windows" {
		return installWindows(cfg)
	}
	return installUnix(cfg)
}

func installUnix(cfg installConfig) (installResult, error) {
	binDir := filepath.Join(cfg.home, ".local", "bin")
	destination := filepath.Join(binDir, "mm")
	configHome := cfg.configHome
	if configHome == "" {
		configHome = filepath.Join(cfg.home, ".config")
	}
	profiles := [...]profileSpec{
		{path: filepath.Join(cfg.home, ".bashrc"), body: posixPathBlock},
		{path: filepath.Join(cfg.home, ".zshrc"), body: posixPathBlock},
		{path: filepath.Join(configHome, "fish", "conf.d", "mm.fish"), body: fishPathBlock},
		{path: filepath.Join(configHome, "powershell", "profile.ps1"), body: powershellPathBlock},
		{path: filepath.Join(configHome, "nushell", "env.nu"), body: nushellPathBlock},
	}
	plans := make([]profilePlan, len(profiles))
	for i, profile := range profiles {
		plan, err := planProfileBlock(profile.path, profile.body)
		if err != nil {
			return installResult{}, err
		}
		plans[i] = plan
	}
	binarySnapshot, err := snapshotFile(destination)
	if err != nil {
		return installResult{}, fmt.Errorf("snapshot %s: %w", destination, err)
	}
	changed, err := copyExecutableIfChanged(cfg.executable, destination, cfg.goos)
	if err != nil {
		return installResult{}, err
	}
	write := cfg.writeFile
	if write == nil {
		write = writeFileAtomic
	}

	result := installResult{
		path:          destination,
		binaryChanged: changed,
		reloadNeeded:  !pathContains(cfg.currentPath, binDir, cfg.goos),
	}
	committed := 0
	for i := range plans {
		if !plans[i].changed {
			continue
		}
		if err := write(plans[i].snapshot.path, plans[i].updated, plans[i].snapshot.mode, false); err != nil {
			rollbackErr := rollbackUnixInstall(plans, committed, binarySnapshot, changed)
			if rollbackErr != nil {
				return installResult{}, fmt.Errorf("update %s: %w; rollback failed: %v", profiles[i].path, err, rollbackErr)
			}
			return installResult{}, fmt.Errorf("update %s: %w", profiles[i].path, err)
		}
		committed = i + 1
		result.profileChanges++
	}
	return result, nil
}

func installWindows(cfg installConfig) (installResult, error) {
	if cfg.localAppData == "" {
		return installResult{}, errors.New("LOCALAPPDATA is not set")
	}
	destination := filepath.Join(cfg.localAppData, "mm", "bin", "mm.exe")
	binarySnapshot, err := snapshotFile(destination)
	if err != nil {
		return installResult{}, fmt.Errorf("snapshot %s: %w", destination, err)
	}
	changed, err := copyExecutableIfChanged(cfg.executable, destination, cfg.goos)
	if err != nil {
		return installResult{}, err
	}

	run := cfg.runCommand
	if run == nil {
		run = runInstallCommand
	}
	if err := run("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", windowsPathScript(filepath.Dir(destination))); err != nil {
		if changed {
			if rollbackErr := restoreSnapshot(binarySnapshot); rollbackErr != nil {
				return installResult{}, fmt.Errorf("configure Windows user PATH: %w; restore %s: %v", err, destination, rollbackErr)
			}
		}
		return installResult{}, fmt.Errorf("configure Windows user PATH: %w", err)
	}
	return installResult{
		path:          destination,
		binaryChanged: changed,
		reloadNeeded:  !pathContains(cfg.currentPath, filepath.Dir(destination), cfg.goos),
	}, nil
}

func pathContains(pathValue, directory, goos string) bool {
	for _, entry := range filepath.SplitList(pathValue) {
		if goos == "windows" {
			if strings.EqualFold(filepath.Clean(entry), filepath.Clean(directory)) {
				return true
			}
			continue
		}
		if filepath.Clean(entry) == filepath.Clean(directory) {
			return true
		}
	}
	return false
}

const posixPathBlock = `case ":${PATH}:" in
  *":${HOME}/.local/bin:"*) ;;
  *) export PATH="${HOME}/.local/bin:${PATH}" ;;
esac`

const fishPathBlock = `fish_add_path --prepend "$HOME/.local/bin"`

const powershellPathBlock = `$mmBin = Join-Path $HOME '.local/bin'
if (($env:PATH -split [IO.Path]::PathSeparator) -notcontains $mmBin) {
    $env:PATH = "$mmBin$([IO.Path]::PathSeparator)$env:PATH"
}`

const nushellPathBlock = `let mm_bin = ($nu.home-path | path join '.local' 'bin')
if $mm_bin not-in $env.PATH {
    $env.PATH = ($env.PATH | prepend $mm_bin)
}`

func windowsPathScript(binDir string) string {
	escaped := strings.ReplaceAll(binDir, "'", "''")
	return fmt.Sprintf(`$mmBin = '%s'; $current = [Environment]::GetEnvironmentVariable('Path', 'User'); $parts = if ([string]::IsNullOrEmpty($current)) { @() } else { $current -split [IO.Path]::PathSeparator }; if ($parts -notcontains $mmBin) { $next = if ([string]::IsNullOrEmpty($current)) { $mmBin } else { "$mmBin$([IO.Path]::PathSeparator)$current" }; [Environment]::SetEnvironmentVariable('Path', $next, 'User') }`, escaped)
}

func runInstallCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ensureProfileBlock(path, body string) (bool, error) {
	plan, err := planProfileBlock(path, body)
	if err != nil {
		return false, err
	}
	if !plan.changed {
		return false, nil
	}
	if err := writeFileAtomic(plan.snapshot.path, plan.updated, plan.snapshot.mode, false); err != nil {
		return false, fmt.Errorf("update %s: %w", path, err)
	}
	return true, nil
}

func planProfileBlock(path, body string) (profilePlan, error) {
	writePath, err := profileWritePath(path)
	if err != nil {
		return profilePlan{}, err
	}
	snapshot, err := snapshotFile(writePath)
	if err != nil {
		return profilePlan{}, err
	}
	data := snapshot.data
	block := []byte(profileMarkerStart + "\n" + body + "\n" + profileMarkerEnd)
	start := bytes.Index(data, []byte(profileMarkerStart))
	end := bytes.Index(data, []byte(profileMarkerEnd))
	if (start >= 0) != (end >= 0) || (start >= 0 && end < start) {
		return profilePlan{}, fmt.Errorf("%s contains an incomplete mm profile block", path)
	}

	var updated []byte
	if start >= 0 {
		end += len(profileMarkerEnd)
		if bytes.Equal(data[start:end], block) {
			return profilePlan{snapshot: snapshot}, nil
		}
		updated = make([]byte, 0, len(data)-end+start+len(block))
		updated = append(updated, data[:start]...)
		updated = append(updated, block...)
		updated = append(updated, data[end:]...)
	} else {
		updated = make([]byte, 0, len(data)+len(block)+2)
		updated = append(updated, data...)
		if len(updated) > 0 {
			if updated[len(updated)-1] != '\n' {
				updated = append(updated, '\n')
			}
			updated = append(updated, '\n')
		}
		updated = append(updated, block...)
		updated = append(updated, '\n')
	}
	return profilePlan{snapshot: snapshot, updated: updated, changed: true}, nil
}

func snapshotFile(path string) (fileSnapshot, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{path: path, mode: 0o644}, nil
	}
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("read %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("inspect %s: %w", path, err)
	}
	return fileSnapshot{path: path, data: data, mode: info.Mode().Perm(), exists: true}, nil
}

func restoreSnapshot(snapshot fileSnapshot) error {
	if snapshot.exists {
		return writeFileAtomic(snapshot.path, snapshot.data, snapshot.mode, false)
	}
	if err := os.Remove(snapshot.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func rollbackUnixInstall(plans []profilePlan, committed int, binary fileSnapshot, binaryChanged bool) error {
	var firstErr error
	for i := committed - 1; i >= 0; i-- {
		if !plans[i].changed {
			continue
		}
		if err := restoreSnapshot(plans[i].snapshot); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if binaryChanged {
		if err := restoreSnapshot(binary); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func profileWritePath(path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve profile symlink %s: %w", path, err)
	}
	return resolved, nil
}

func copyExecutableIfChanged(source, destination, goos string) (bool, error) {
	equal, err := filesEqual(source, destination)
	if err != nil {
		return false, err
	}
	if equal {
		if goos != "windows" {
			info, err := os.Stat(destination)
			if err != nil {
				return false, fmt.Errorf("inspect %s: %w", destination, err)
			}
			if info.Mode().Perm()&0o111 == 0 {
				if err := os.Chmod(destination, info.Mode().Perm()|0o755); err != nil {
					return false, fmt.Errorf("make %s executable: %w", destination, err)
				}
				return true, nil
			}
		}
		return false, nil
	}

	sourceFile, err := os.Open(source)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", source, err)
	}
	defer sourceFile.Close()
	if err := writeReaderAtomic(destination, sourceFile, 0o755, goos == "windows"); err != nil {
		return false, fmt.Errorf("install %s: %w", destination, err)
	}
	return true, nil
}

func filesEqual(leftPath, rightPath string) (bool, error) {
	leftInfo, err := os.Stat(leftPath)
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", leftPath, err)
	}
	rightInfo, err := os.Stat(rightPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", rightPath, err)
	}
	if leftInfo.Size() != rightInfo.Size() {
		return false, nil
	}

	left, err := os.Open(leftPath)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", leftPath, err)
	}
	defer left.Close()
	right, err := os.Open(rightPath)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", rightPath, err)
	}
	defer right.Close()

	var leftBuffer [32 * 1024]byte
	var rightBuffer [32 * 1024]byte
	for {
		leftN, leftErr := left.Read(leftBuffer[:])
		rightN, rightErr := right.Read(rightBuffer[:])
		if leftN != rightN || !bytes.Equal(leftBuffer[:leftN], rightBuffer[:rightN]) {
			return false, nil
		}
		if errors.Is(leftErr, io.EOF) && errors.Is(rightErr, io.EOF) {
			return true, nil
		}
		if leftErr != nil {
			return false, fmt.Errorf("read %s: %w", leftPath, leftErr)
		}
		if rightErr != nil {
			return false, fmt.Errorf("read %s: %w", rightPath, rightErr)
		}
	}
}

func writeReaderAtomic(path string, source io.Reader, mode os.FileMode, replace bool) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".mm-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		temporary.Close()
		os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := io.Copy(temporary, source); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if replace {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Rename(temporaryPath, path)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode, replace bool) error {
	return writeReaderAtomic(path, bytes.NewReader(data), mode, replace)
}
