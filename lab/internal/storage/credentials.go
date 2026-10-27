package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/models"
)

// FileCredentials stores secrets outside graph documents and the served web tree.
// The file is owner-readable/writable (0600); it is not encrypted at rest.
type FileCredentials struct{ Path string }

func (s FileCredentials) Load() (models.Credentials, error) {
	var credentials models.Credentials
	info, err := os.Lstat(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return credentials, nil
	}
	if err != nil {
		return credentials, errors.New("could not inspect credential storage")
	}
	if !info.Mode().IsRegular() {
		return credentials, errors.New("credential storage must be a regular file")
	}
	if err := os.Chmod(s.Path, 0600); err != nil {
		return credentials, errors.New("could not restrict credential file permissions")
	}
	file, err := os.Open(s.Path)
	if err != nil {
		return credentials, errors.New("could not open credential storage")
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&credentials); err != nil {
		return models.Credentials{}, errors.New("invalid credential storage")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return models.Credentials{}, errors.New("invalid credential storage")
	}
	return credentials, nil
}
func (s FileCredentials) Save(credentials models.Credentials) error {
	directory := filepath.Dir(s.Path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".model-credentials-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if err := json.NewEncoder(file).Encode(credentials); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, s.Path); err != nil {
		return fmt.Errorf("replace credential file: %w", err)
	}
	return nil
}
