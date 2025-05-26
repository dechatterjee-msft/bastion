package worker

import (
	"context"
	"fmt"

	"github.com/bastion/internal/hash"
	"github.com/bastion/internal/storage"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type BackupEvent struct {
	Object  *unstructured.Unstructured
	Deleted bool
}

type BackupWorker struct {
	GVK        schema.GroupVersionKind
	Queue      chan BackupEvent
	Hasher     hash.Hasher
	Store      storage.StorageWriter
	MaxRetries int
}

func NewBackupWorker(gvk schema.GroupVersionKind, hasher hash.Hasher, store storage.StorageWriter, queueSize, maxRetries int) *BackupWorker {
	return &BackupWorker{
		GVK:        gvk,
		Queue:      make(chan BackupEvent, queueSize),
		Hasher:     hasher,
		Store:      store,
		MaxRetries: maxRetries,
	}
}

func (bw *BackupWorker) Run(ctx context.Context) {
	for event := range bw.Queue {
		retries := 0
		for retries < bw.MaxRetries {
			if event.Deleted {
				if err := bw.Store.Delete(ctx, bw.GVK, event.Object.GetNamespace(), event.Object.GetName()); err != nil {
					fmt.Printf("[%s] delete failed: %v (retry %d/%d)\n", bw.GVK.Kind, err, retries+1, bw.MaxRetries)
					retries++
					continue
				}
				break
			}

			hashStr, err := bw.Hasher.Hash(event.Object)
			if err != nil {
				fmt.Printf("[%s] hash failed: %v (retry %d/%d)\n", bw.GVK.Kind, err, retries+1, bw.MaxRetries)
				retries++
				continue
			}

			_, oldHash, err := bw.Store.Read(ctx, bw.GVK, event.Object.GetNamespace(), event.Object.GetName())
			if err != nil {
				fmt.Printf("[%s] read failed: %v (retry %d/%d)\n", bw.GVK.Kind, err, retries+1, bw.MaxRetries)
				retries++
				continue
			}

			if hashStr == oldHash {
				break // no change
			}

			if _, err := bw.Store.Write(ctx, event.Object, hashStr); err != nil {
				fmt.Printf("[%s] write failed: %v (retry %d/%d)\n", bw.GVK.Kind, err, retries+1, bw.MaxRetries)
				retries++
				continue
			}
			fmt.Printf("[%s] backup successful for %s/%s\n", bw.GVK.Kind, event.Object.GetNamespace(), event.Object.GetName())
			break
		}
		if retries == bw.MaxRetries {
			fmt.Printf("[%s] giving up on %s/%s after %d retries\n", bw.GVK.Kind, event.Object.GetNamespace(), event.Object.GetName(), bw.MaxRetries)
		}
	}
}

func (bw *BackupWorker) Enqueue(obj *unstructured.Unstructured, deleted bool) {
	bw.Queue <- BackupEvent{Object: obj, Deleted: deleted}
}
