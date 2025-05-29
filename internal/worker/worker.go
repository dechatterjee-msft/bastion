package worker

import (
	"context"
	"fmt"
	"github.com/bastion/internal/hash"
	"github.com/bastion/internal/storage"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
)

type BackupEvent struct {
	Object    *unstructured.Unstructured
	EventType EventType
	GVK       schema.GroupVersionKind
}

type EventType int

const (
	Update = iota
	Delete
	Create
)

type BackupWorker struct {
	Name        string
	Queue       chan BackupEvent
	Hasher      hash.Hasher
	Store       storage.Storage
	MaxRetries  int
	processed   int
	WorkerCount int
}

func NewBackupWorker(name string, hasher hash.Hasher, store storage.Storage, queueSize, maxRetries, workerCount int) *BackupWorker {
	return &BackupWorker{
		Name:        fmt.Sprintf("worker-%s", name),
		Queue:       make(chan BackupEvent, queueSize),
		Hasher:      hasher,
		Store:       store,
		MaxRetries:  maxRetries,
		WorkerCount: workerCount,
	}
}

func (bw *BackupWorker) StartWorkers(ctx context.Context) {
	for i := 0; i < bw.WorkerCount; i++ {
		go func(id int) {
			for {
				select {
				case <-ctx.Done():
					return
				case event := <-bw.Queue:
					logger := log.FromContext(ctx).WithName("BackupWorker").
						WithName(strconv.Itoa(id)).
						WithValues("namespace",
							event.Object.GetNamespace(),
							"name", event.Object.GetName(),
							"kind", event.GVK.Kind,
							"eventType", event.EventType)
					bw.processed++
					retries := 0
					for retries < bw.MaxRetries {
						switch event.EventType {
						case Delete:
							logger.Info("Backup delete event triggered")
							if err := bw.Store.MarkTombstone(ctx, event.GVK, event.Object.GetNamespace(), event.Object.GetName()); err != nil {
								logger.Error(err, "failed to delete object", "currentRetry", retries+1, "maxRetries", bw.MaxRetries)
								retries++
								continue
							}
							break
						case Update:
							// TODO can handle anything special with Update
							logger.Info("Backup update event triggered")
						case Create:
							// TODO can handle anything special with Create
							logger.Info("Backup create event triggered")
						default:
							logger.Info("bad event triggered")
						}
						hashStr, err := bw.Hasher.Hash(event.Object)
						if err != nil {
							logger.Error(err, "failed to hash", "currentRetry", retries+1, "maxRetries", bw.MaxRetries)
							retries++
							continue
						}
						_, oldHash, err := bw.Store.Read(ctx, event.GVK, event.Object.GetNamespace(), event.Object.GetName())
						if err != nil {
							logger.Error(err, "failed to read file", "currentRetry", retries+1, "maxRetries", bw.MaxRetries)
							retries++
							continue
						}
						if hashStr == oldHash {
							break // no change
						}
						if _, err := bw.Store.Write(ctx, event.Object, hashStr); err != nil {
							logger.Error(err, "write failed")
							retries++
							continue
						}
						logger.Info("backup successful")
						break
					}
					if retries == bw.MaxRetries {
						logger.Info("backup retries exceeded", "currentRetry", retries+1, "maxRetries", bw.MaxRetries)
					}
				}
			}
		}(i)
	}
}

func (bw *BackupWorker) Stats() map[string]interface{} {
	return map[string]interface{}{
		"worker":    bw.Name,
		"processed": bw.processed,
		"queueLen":  len(bw.Queue),
	}
}

func (bw *BackupWorker) Enqueue(obj *unstructured.Unstructured, eventType EventType) {
	bw.Queue <- BackupEvent{Object: obj, EventType: eventType, GVK: obj.GroupVersionKind()}
}
