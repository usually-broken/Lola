// Package home resolves the Lola home directory and the paths inside it.
//
// Everything Lola needs at runtime lives under one folder: the binary, the
// config, and the sessions, so there is one place to look, one place to back up
// and one place to delete. That folder is ~/.lola by default, or wherever
// LOLA_HOME points.
package home

import (
	"os"
	"path/filepath"
)

// DirName is the home folder's name in the user's directory.
const DirName = ".lola"

// EnvVar overrides the home directory when set.
const EnvVar = "LOLA_HOME"

// Dir returns the Lola home directory. It is LOLA_HOME when set, otherwise
// ~/.lola.
func Dir() string {
	if dir := os.Getenv(EnvVar); dir != "" {
		return dir
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		// No home directory: fall back to the current directory, so the
		// harness still runs rather than failing to start.
		return DirName
	}
	return filepath.Join(userHome, DirName)
}

// ConfigPath is where lola.yaml lives: the home directory.
func ConfigPath() string { return filepath.Join(Dir(), "lola.yaml") }

// ProvidersPath is where providers added from an interface are kept. It lives
// with the harness rather than in the repo, so nothing written by clicking a
// button is ever committed.
func ProvidersPath() string { return filepath.Join(Dir(), "providers.yaml") }

// SettingsPath is where choices made from an interface are kept: the model, the
// mode. Same reason as the providers file — a button must never write into the
// repo — and same rule about what belongs in it: choices, not secrets.
func SettingsPath() string { return filepath.Join(Dir(), "settings.json") }

// SessionsDir is where saved sessions live: a folder inside the home directory.
func SessionsDir() string { return filepath.Join(Dir(), "sessions") }

// WorkspaceDir is the default place an agent's file tools may touch: a folder
// in the home directory, so nothing scribbles outside it by default.
func WorkspaceDir() string { return filepath.Join(Dir(), "workspace") }

// BinDir is where an installed binary lives: a bin folder in the home
// directory.
func BinDir() string { return filepath.Join(Dir(), "bin") }

// BinaryPath is the full path to an installed lola binary.
func BinaryPath() string { return filepath.Join(BinDir(), "lola") }

// Ensure creates the home directory if it does not exist.
func Ensure() error {
	return os.MkdirAll(Dir(), 0o755)
}
