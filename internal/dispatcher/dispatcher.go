package dispatcher

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sync"

	"github.com/bastion/internal/worker"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

type Dispatcher struct {
	informerCancels map[string]context.CancelFunc
	mu              sync.Mutex
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		informerCancels: make(map[string]context.CancelFunc),
	}
}

func (d *Dispatcher) Register(ctx context.Context, gvr schema.GroupVersionResource, gvk schema.GroupVersionKind, dynamicClient dynamic.Interface, w *worker.BackupWorker) error {
	logger := log.FromContext(ctx)
	logger.Info("Registering backup controller", "gvr", gvr.String(), "gvk", gvk.String())
	// Create a tweakListOptions function to filter by label
	tweakListOptions := func(opts *metav1.ListOptions) {
		// opts.LabelSelector = "backup.bastion.io/enabled=true"
	}
	filteredInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, 0, metav1.NamespaceAll, tweakListOptions)
	informer := filteredInformerFactory.ForResource(gvr).Informer()
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			d.enqueueIfAnnotated(obj, w, 2)
		},
		UpdateFunc: func(_, newObj interface{}) {
			d.enqueueIfAnnotated(newObj, w, 0)
		},
		DeleteFunc: func(obj interface{}) {
			d.enqueueIfAnnotated(obj, w, 1)
		},
	})
	if err != nil {
		return err
	}
	d.mu.Lock()
	childCtx, cancel := context.WithCancel(ctx)
	key := gvk.String()
	d.informerCancels[key] = cancel
	d.mu.Unlock()
	go informer.Run(childCtx.Done())
	return nil
}

func (d *Dispatcher) Stop(ctx context.Context, gvk schema.GroupVersionKind) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	logger := log.FromContext(ctx)
	if cancel, ok := d.informerCancels[gvk.String()]; ok {
		cancel()
		delete(d.informerCancels, gvk.String())
	}
	logger.Info("Stopping informer", "gvk", gvk.String())
	return nil
}

func gvkKey(gvk schema.GroupVersionKind) string {
	return fmt.Sprintf("%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind)
}

func (d *Dispatcher) enqueueIfAnnotated(obj interface{}, w *worker.BackupWorker, eventType worker.EventType) {
	var (
		u *unstructured.Unstructured
	)
	if eventType == 0 {
		// Handle tombstone
		if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
			if realObj, ok := tombstone.Obj.(*unstructured.Unstructured); ok {
				u = realObj
			} else {
				return
			}
		} else if o, ok := obj.(*unstructured.Unstructured); ok {
			u = o
		} else {
			return
		}
	} else {
		u = obj.(*unstructured.Unstructured)
	}
	w.Enqueue(u.DeepCopy(), eventType)
}
