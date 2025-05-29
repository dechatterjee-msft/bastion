package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"os"
	"path/filepath"
	"sync"
)

// Storage defines the interface for any backup storage backend.
type Storage interface {
	Write(ctx context.Context, obj *unstructured.Unstructured, hash string) (changed bool, err error)
	Read(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, string, error)
	Delete(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) error
	MarkTombstone(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) error
}

// FileSystemWriter writes backup data to the local filesystem default storage implementation
type FileSystemWriter struct {
	BaseDir string
}

// writerCache holds cached writers and synchronization
var (
	writerCache = make(map[string]*FileSystemWriter)
	writerMu    sync.Mutex
)

// NewFileSystemWriter creates a new file-system based writer.
func NewFileSystemWriter(baseDir string) *FileSystemWriter {
	writerMu.Lock()
	if baseDir == "" {
		baseDir = os.TempDir()
	}
	defer writerMu.Unlock()
	if w, ok := writerCache[baseDir]; ok {
		return w
	}
	w := &FileSystemWriter{BaseDir: baseDir}
	writerCache[baseDir] = w
	return w
}

// Write stores manifest and hash.txt for the given object.
func (w *FileSystemWriter) Write(ctx context.Context, obj *unstructured.Unstructured, hash string) (bool, error) {
	gvk := obj.GroupVersionKind()
	dir := filepath.Join(w.BaseDir, gvk.Group, gvk.Version, gvk.Kind, obj.GetNamespace(), obj.GetName())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false, fmt.Errorf("failed to create backup dir: %w", err)
	}
	hashPath := filepath.Join(dir, "hash.txt")
	manifestPath := filepath.Join(dir, "manifest.yaml")
	oldHash, _ := os.ReadFile(hashPath)
	if string(oldHash) == hash {
		return false, nil // no change
	}
	if err := os.WriteFile(hashPath, []byte(hash), 0644); err != nil {
		return false, fmt.Errorf("failed to write hash: %w", err)
	}
	data, err := json.MarshalIndent(obj.Object, "", "  ")
	if err != nil {
		return false, fmt.Errorf("failed to marshal object: %w", err)
	}

	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		return false, fmt.Errorf("failed to write manifest: %w", err)
	}

	return true, nil
}

// Read loads a CR's manifest and hash from the filesystem.
func (w *FileSystemWriter) Read(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, string, error) {
	dir := filepath.Join(w.BaseDir, gvk.Group, gvk.Version, gvk.Kind, namespace, name)
	hashPath := filepath.Join(dir, "hash.txt")
	manifestPath := filepath.Join(dir, "manifest.yaml")
	hashBytes, err := os.ReadFile(hashPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("failed to read hash: %w", err)
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("failed to read manifest: %w", err)
	}
	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal(manifestBytes, &obj.Object); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal manifest: %w", err)
	}
	obj.SetGroupVersionKind(gvk)
	return obj, string(hashBytes), nil
}

func (w *FileSystemWriter) Delete(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) error {
	dir := filepath.Join(w.BaseDir, gvk.Group, gvk.Version, gvk.Kind, namespace, name)
	writerMu.Lock()
	defer writerMu.Unlock()
	delete(writerCache, w.BaseDir)
	return os.RemoveAll(dir)
}

func (w *FileSystemWriter) MarkTombstone(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) error {
	orig := filepath.Join(w.BaseDir, gvk.Group, gvk.Version, gvk.Kind, namespace, name)
	tomb := filepath.Join(w.BaseDir, gvk.Group, gvk.Version, gvk.Kind, namespace, name, "tombstone")
	// Check if original path exists
	if _, err := os.Stat(orig); os.IsNotExist(err) {
		return fmt.Errorf("cannot mark tombstone: original path does not exist: %s", orig)
	}
	_, err := os.Create(tomb)
	if err != nil {
		return err
	}
	return nil
}
