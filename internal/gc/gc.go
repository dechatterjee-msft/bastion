package gc

import (
	"context"
	"github.com/bastion/internal/storage"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
	"time"
)

type GarbageCollector struct {
	BaseDir       string
	RetainPeriod  time.Duration
	DynamicClient dynamic.Interface
	Store         storage.Storage
}

func NewGarbageCollector(retain time.Duration,
	dynamicClient dynamic.Interface,
	store storage.Storage) *GarbageCollector {
	return &GarbageCollector{
		RetainPeriod:  retain,
		DynamicClient: dynamicClient,
		Store:         store,
	}
}

func (gc *GarbageCollector) Run(ctx context.Context) {
	logger := log.FromContext(ctx).WithName("GarbageCollector").WithName("run")
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

	tombstones, err := gc.Store.ListTombstones(ctx)
	if err != nil {
		logger.Error(err, "failed to list tombstones")
		return
	}
	for _, entry := range tombstones {
		age := time.Since(entry.ModTime)
		if age > gc.RetainPeriod {
			_, _, err := gc.Store.Read(ctx, entry.GVK, entry.Namespace, entry.Name)
			if err != nil {
				logger.Error(err, "failed to read object from storage", "gvk", entry.GVK, "ns", entry.Namespace, "name", entry.Name)
				continue
			}

			gvr := schema.GroupVersionResource{
				Group:    entry.GVK.Group,
				Version:  entry.GVK.Version,
				Resource: strings.ToLower(entry.GVK.Kind) + "s", // same: simple plural for now
			}

			res := gc.DynamicClient.Resource(gvr).Namespace(entry.Namespace)
			_, err = res.Get(ctx, entry.Name, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					logger.Info("Cleaning tombstoned object", "gvk", entry.GVK, "ns", entry.Namespace, "name", entry.Name)
					_ = gc.Store.Delete(ctx, entry.GVK, entry.Namespace, entry.Name)
				} else {
					logger.Error(err, "error checking resource existence", "gvk", entry.GVK, "ns", entry.Namespace, "name", entry.Name)
				}
			} else {
				_ = gc.Store.DeleteTombstone(ctx, entry.GVK, entry.Namespace, entry.Name)
			}
		}
	}
}
