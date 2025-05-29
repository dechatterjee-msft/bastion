package gc

import (
	"context"
	"os"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
	"time"
)

type GarbageCollector struct {
	BaseDir      string
	RetainPeriod time.Duration
}

func NewGarbageCollector(baseDir string, retain time.Duration) *GarbageCollector {
	return &GarbageCollector{
		BaseDir:      baseDir,
		RetainPeriod: retain,
	}
}

func (gc *GarbageCollector) Run(ctx context.Context) {
	logger := log.FromContext(ctx).WithName("GarbageCollector")
	logger.Info("Starting garbage collector")
	ticker := time.NewTicker(gc.RetainPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Garbage collector stopped")
			return
		case <-ticker.C:
			gc.sweep(ctx)
		}
	}
}

func (gc *GarbageCollector) sweep(ctx context.Context) {
	logger := log.FromContext(ctx).WithName("GarbageCollector").WithName("sweep")

	err := filepath.Walk(gc.BaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}

		// Only process files
		if info.IsDir() || !strings.HasSuffix(info.Name(), "tombstone") {
			return nil
		}

		age := time.Since(info.ModTime())
		if age > gc.RetainPeriod {
			parentDir := filepath.Dir(path)
			logger.Info("Cleaning up tombstoned resource", "dir", parentDir, "age", age)
			if rmErr := os.RemoveAll(parentDir); rmErr != nil {
				logger.Error(rmErr, "failed to delete tombstoned directory", "dir", parentDir)
			}
		}

		return nil
	})

	if err != nil {
		logger.Error(err, "garbage collection sweep failed")
	}
}
