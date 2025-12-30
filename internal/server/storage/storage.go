package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/config"
)

// Storage interface for artifact storage
type Storage interface {
	// Save stores an artifact and returns the path
	Save(product, version, filename string, reader io.Reader) (string, error)
	// Get returns a reader for an artifact
	Get(product, version, filename string) (io.ReadCloser, error)
	// Delete removes an artifact
	Delete(product, version, filename string) error
	// Exists checks if an artifact exists
	Exists(product, version, filename string) bool
	// GetPath returns the full path to an artifact
	GetPath(product, version, filename string) string
}

// LocalStorage implements Storage for local filesystem
type LocalStorage struct {
	basePath string
}

// New creates a new storage instance based on configuration
func New(cfg config.StorageConfig) (Storage, error) {
	switch cfg.Type {
	case "local":
		return NewLocalStorage(cfg.LocalPath)
	case "s3":
		return nil, fmt.Errorf("S3 storage not implemented yet")
	default:
		return nil, fmt.Errorf("unknown storage type: %s", cfg.Type)
	}
}

// NewLocalStorage creates a new local filesystem storage
func NewLocalStorage(basePath string) (*LocalStorage, error) {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return &LocalStorage{basePath: basePath}, nil
}

// Save stores an artifact
func (s *LocalStorage) Save(product, version, filename string, reader io.Reader) (string, error) {
	dir := filepath.Join(s.basePath, product, version)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	path := filepath.Join(dir, filename)
	file, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return path, nil
}

// Get returns a reader for an artifact
func (s *LocalStorage) Get(product, version, filename string) (io.ReadCloser, error) {
	path := s.GetPath(product, version, filename)
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	return file, nil
}

// Delete removes an artifact
func (s *LocalStorage) Delete(product, version, filename string) error {
	path := s.GetPath(product, version, filename)
	return os.Remove(path)
}

// Exists checks if an artifact exists
func (s *LocalStorage) Exists(product, version, filename string) bool {
	path := s.GetPath(product, version, filename)
	_, err := os.Stat(path)
	return err == nil
}

// GetPath returns the full path to an artifact
func (s *LocalStorage) GetPath(product, version, filename string) string {
	return filepath.Join(s.basePath, product, version, filename)
}

