package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/lichenghsu/synccraft/internal/conn"
)

// FileKind selects the dialog type and file filters for PickFile.
type FileKind string

const (
	FileSQLiteOpen    FileKind = "sqliteOpen"
	FileSQLiteCreate  FileKind = "sqliteCreate"
	FileCACert        FileKind = "caCert"
	FileClientCert    FileKind = "clientCert"
	FileClientKey     FileKind = "clientKey"
	FileSSHPrivateKey FileKind = "sshPrivateKey"
	FileKnownHosts    FileKind = "knownHosts"
)

var (
	ErrUnknownFileKind = errors.New("unknown file kind")
	ErrPathNotPicked   = errors.New("file path was not chosen through a file dialog")
)

type dialogSpec struct {
	title   string
	save    bool
	inSSH   bool // start in ~/.ssh and show hidden files
	filters []application.FileFilter
}

var (
	pemFilter = application.FileFilter{DisplayName: "PEM files (*.pem, *.crt, *.key)", Pattern: "*.pem;*.crt;*.cer;*.key"}
	anyFilter = application.FileFilter{DisplayName: "All files", Pattern: "*"}
	dbFilter  = application.FileFilter{DisplayName: "SQLite databases", Pattern: "*.db;*.sqlite;*.sqlite3"}
)

var dialogSpecs = map[FileKind]dialogSpec{
	FileSQLiteOpen:    {title: "Open SQLite database", filters: []application.FileFilter{dbFilter, anyFilter}},
	FileSQLiteCreate:  {title: "Create SQLite database", save: true, filters: []application.FileFilter{dbFilter}},
	FileCACert:        {title: "Choose CA certificate", filters: []application.FileFilter{pemFilter, anyFilter}},
	FileClientCert:    {title: "Choose client certificate", filters: []application.FileFilter{pemFilter, anyFilter}},
	FileClientKey:     {title: "Choose client key", filters: []application.FileFilter{pemFilter, anyFilter}},
	FileSSHPrivateKey: {title: "Choose SSH private key", inSSH: true, filters: []application.FileFilter{anyFilter}},
	FileKnownHosts:    {title: "Choose known_hosts file", inSSH: true, filters: []application.FileFilter{anyFilter}},
}

// FileService opens native file dialogs and remembers which paths the user
// chose. Other services accept only those paths, so the frontend cannot make
// the backend open arbitrary files.
type FileService struct {
	mu      sync.Mutex
	allowed map[string]struct{}
	prompt  func(dialogSpec) (string, error)
}

func NewFileService() *FileService {
	return &FileService{allowed: map[string]struct{}{}, prompt: showDialog}
}

// PickFile opens a native file dialog for kind and returns the chosen path,
// or "" if the user cancelled.
func (s *FileService) PickFile(kind FileKind) (string, error) {
	spec, ok := dialogSpecs[kind]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownFileKind, kind)
	}

	p, err := s.prompt(spec)
	if err != nil || p == "" {
		return "", err
	}

	p = filepath.Clean(p)
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("%w: %q", conn.ErrInvalidPath, p)
	}
	s.allow(p)
	return p, nil
}

// allow is unexported on purpose: exported methods become frontend bindings.
func (s *FileService) allow(paths ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range paths {
		s.allowed[filepath.Clean(p)] = struct{}{}
	}
}

// checkPaths fails unless every file path in c was chosen through PickFile
// (or loaded by the backend and passed to allow).
func (s *FileService) checkPaths(c *conn.ConnectionConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range c.FilePaths() {
		if _, ok := s.allowed[p]; !ok {
			return fmt.Errorf("%w: %q", ErrPathNotPicked, p)
		}
	}
	return nil
}

func showDialog(spec dialogSpec) (string, error) {
	app := application.Get()
	if app == nil {
		return "", errors.New("application not running")
	}

	dir := ""
	if spec.inSSH {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, ".ssh")
		}
	}

	if spec.save {
		d := app.Dialog.SaveFile().SetMessage(spec.title).CanCreateDirectories(true)
		for _, f := range spec.filters {
			d.AddFilter(f.DisplayName, f.Pattern)
		}
		if dir != "" {
			d.SetDirectory(dir)
		}
		return d.PromptForSingleSelection()
	}

	d := app.Dialog.OpenFile().SetTitle(spec.title).CanChooseFiles(true).ShowHiddenFiles(spec.inSSH)
	for _, f := range spec.filters {
		d.AddFilter(f.DisplayName, f.Pattern)
	}
	if dir != "" {
		d.SetDirectory(dir)
	}
	return d.PromptForSingleSelection()
}
