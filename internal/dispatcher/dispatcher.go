package dispatcher

import (
	"context"
	"fmt"

	"github.com/bastion/internal/worker"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

type Dispatcher struct {
	Workers map[string]*worker.BackupWorker
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		Workers: make(map[string]*worker.BackupWorker),
	}
}

func (d *Dispatcher) Register(ctx context.Context, gvr schema.GroupVersionResource, gvk schema.GroupVersionKind, informerFactory dynamicinformer.DynamicSharedInformerFactory, w *worker.BackupWorker) error {
	key := gvkKey(gvk)
	if _, exists := d.Workers[key]; exists {
		return nil
	}
	d.Workers[key] = w
	go w.Run(ctx)

	informer := informerFactory.ForResource(gvr).Informer()
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			d.enqueueIfAnnotated(obj, w)
		},
		UpdateFunc: func(_, newObj interface{}) {
			d.enqueueIfAnnotated(newObj, w)
		},
		DeleteFunc: func(obj interface{}) {
			d.enqueueIfAnnotated(obj, w)
		},
	})
	if err != nil {
		return err
	}

	go informer.Run(ctx.Done())
	return nil
}

func (d *Dispatcher) Stop(gvk schema.GroupVersionKind) error {
	key := gvkKey(gvk)
	w, ok := d.Workers[key]
	if !ok {
		return fmt.Errorf("no worker registered for GVK %s", key)
	}
	close(w.Queue) // signal graceful shutdown
	delete(d.Workers, key)
	fmt.Printf("Stopped dispatcher for GVK: %s\n", key)
	return nil
}

func (d *Dispatcher) enqueueIfAnnotated(obj interface{}, w *worker.BackupWorker) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	if u.GetAnnotations()["backup.bastion.io/enabled"] != "true" {
		return
	}
	deleted := u.GetDeletionTimestamp() != nil && len(u.GetFinalizers()) == 0
	w.Enqueue(u.DeepCopy(), deleted)
}

func gvkKey(gvk schema.GroupVersionKind) string {
	return fmt.Sprintf("%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind)
}
