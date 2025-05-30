package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/bastion/internal/storage"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileSystem writes backup data to the local filesystem default storage implementation
type FileSystem struct {
	BaseDir string
}

// writerCache holds cached writers and synchronization
var (
	writerCache = make(map[string]*FileSystem)
	writerMu    sync.Mutex
)

// NewFileSystemBasedBackup creates a new file-system based writer.
func NewFileSystemBasedBackup(baseDir string) *FileSystem {
	writerMu.Lock()
	if baseDir == "" {
		baseDir = os.TempDir()
	}
	defer writerMu.Unlock()
	if w, ok := writerCache[baseDir]; ok {
		return w
	}
	w := &FileSystem{BaseDir: baseDir}
	writerCache[baseDir] = w
	return w
}

// Write stores manifest and hash.txt for the given object.
func (w *FileSystem) Write(ctx context.Context, obj *unstructured.Unstructured, hash string) (bool, error) {
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
func (w *FileSystem) Read(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, string, error) {
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

func (w *FileSystem) Delete(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) error {
	dir := filepath.Join(w.BaseDir, gvk.Group, gvk.Version, gvk.Kind, namespace, name)
	writerMu.Lock()
	defer writerMu.Unlock()
	delete(writerCache, w.BaseDir)
	return os.RemoveAll(dir)
}

func (w *FileSystem) MarkTombstone(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) error {
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

func (w *FileSystem) ListTombstones(ctx context.Context) ([]storage.TombstoneEntry, error) {
	var entries []storage.TombstoneEntry
	err := filepath.Walk(w.BaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, "tombstone") {
			gvk, namespace, name, parseErr := w.parsePathFromFilePath(path, w.BaseDir)
			if parseErr != nil {
				return nil // skip bad entries
			}
			entries = append(entries, storage.TombstoneEntry{
				GVK:       gvk,
				Namespace: namespace,
				Name:      name,
				ModTime:   info.ModTime(),
			})
		}
		return nil
	})
	return entries, err
}

func (w *FileSystem) TombstonePath(gvk schema.GroupVersionKind, namespace, name string) string {
	return filepath.Join(
		w.BaseDir,
		gvk.Group,
		gvk.Version,
		gvk.Kind,
		namespace,
		name,
		"tombstone",
	)
}

func (w *FileSystem) DeleteTombstone(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) error {
	tombstonePath := w.TombstonePath(gvk, namespace, name)
	return os.Remove(tombstonePath)
}

func (w *FileSystem) parsePathFromFilePath(path string, baseDir string) (schema.GroupVersionKind, string, string, error) {
	relPath, err := filepath.Rel(baseDir, filepath.Dir(path))
	if err != nil {
		return schema.GroupVersionKind{}, "", "", err
	}
	parts := strings.Split(relPath, string(os.PathSeparator))
	if len(parts) < 5 {
		return schema.GroupVersionKind{}, "", "", fmt.Errorf("invalid path format")
	}
	gvk := schema.GroupVersionKind{
		Group:   parts[0],
		Version: parts[1],
		Kind:    parts[2],
	}
	namespace := parts[3]
	name := parts[4]
	return gvk, namespace, name, nil
}
