package tokenstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	dirMode  os.FileMode = 0o700
	fileMode os.FileMode = 0o600
)

type State struct {
	AccessToken    string    `json:"accessToken"`
	RefreshToken   string    `json:"refreshToken"`
	AccessTokenExp time.Time `json:"accessTokenExp"`
	AuthEmail      string    `json:"authEmail"`
	AuthServiceURL string    `json:"authServiceURL"`
	WrittenAt      time.Time `json:"writtenAt"`
}

type FileStore struct {
	path string
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Load(ctx context.Context) (State, bool, error) {
	if err := ctx.Err(); err != nil {
		return State{}, false, err
	}

	info, err := os.Stat(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, false, nil
		}
		return State{}, false, fmt.Errorf("stat token state: %w", err)
	}
	if info.Mode().Perm() != fileMode {
		return State{}, false, fmt.Errorf("token state file has unsafe permissions %s, want %s: %w", info.Mode().Perm(), fileMode, os.ErrPermission)
	}

	file, err := os.Open(s.path)
	if err != nil {
		return State{}, false, fmt.Errorf("open token state: %w", err)
	}
	defer file.Close()

	var state State
	if err := json.NewDecoder(file).Decode(&state); err != nil {
		return State{}, false, fmt.Errorf("decode token state: %w", err)
	}
	return state, true, nil
}

func (s *FileStore) Save(ctx context.Context, state State) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("create token state dir: %w", err)
	}
	if err := os.Chmod(dir, dirMode); err != nil {
		return fmt.Errorf("chmod token state dir: %w", err)
	}

	tempFile, err := os.CreateTemp(dir, ".tokens-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp token state: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	if err := os.Chmod(tempPath, fileMode); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod temp token state: %w", err)
	}
	if err := json.NewEncoder(tempFile).Encode(state); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write token state: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp token state: %w", err)
	}
	if err := os.Chmod(tempPath, fileMode); err != nil {
		return fmt.Errorf("chmod temp token state: %w", err)
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		return fmt.Errorf("rename token state: %w", err)
	}
	cleanup = false
	return nil
}
