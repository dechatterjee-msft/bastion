package controllers

//
//import (
//	"context"
//	"fmt"
//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
//	"path/filepath"
//	"sync"
//
//	"github.com/bastion/internal/hash"
//	"github.com/bastion/internal/storage"
//	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
//	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
//	apiextensionsinformer "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
//	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
//	"k8s.io/apimachinery/pkg/runtime"
//	"k8s.io/apimachinery/pkg/runtime/schema"
//	"k8s.io/client-go/discovery"
//	"k8s.io/client-go/dynamic"
//	"k8s.io/client-go/dynamic/dynamicinformer"
//	"k8s.io/client-go/tools/cache"
//	"sigs.k8s.io/controller-runtime/pkg/client"
//	"sigs.k8s.io/controller-runtime/pkg/manager"
//)
//
//// BackupReconciler listens for CR changes dynamically with GVK-based worker isolation.
//type BackupReconciler struct {
//	Client        client.Client
//	DynamicClient dynamic.Interface
//	Discovery     discovery.DiscoveryInterface
//	Storage       storage.Storage
//	BackupRoot    string
//	Scheme        *runtime.Scheme
//	workers       map[string]chan backupEvent
//	mu            sync.Mutex
//	workerCancels map[string]context.CancelFunc
//	writerCache   map[string]*storage.FileSystemWriter
//	writerMu      sync.Mutex
//}
//
//type backupEvent struct {
//	Object  *unstructured.Unstructured
//	Deleted bool
//}
//
//func NewBackupReconciler(mgr manager.Manager, writer storage.Storage, bkpRoot string) *BackupReconciler {
//
//	return &BackupReconciler{
//		Client:        mgr.GetClient(),
//		DynamicClient: dynamic.NewForConfigOrDie(mgr.GetConfig()),
//		Discovery:     discovery.NewDiscoveryClientForConfigOrDie(mgr.GetConfig()),
//		Storage:       writer,
//		BackupRoot:    bkpRoot,
//		Scheme:        mgr.GetScheme(),
//		workers:       make(map[string]chan backupEvent),
//		workerCancels: make(map[string]context.CancelFunc),
//		writerCache:   make(map[string]*storage.FileSystemWriter),
//	}
//}
//
//func (r *BackupReconciler) SetupWithCrdInformerFactory(ctx context.Context, mgr manager.Manager) error {
//	apiExtClient, err := apiextensionsclientset.NewForConfig(mgr.GetConfig())
//	if err != nil {
//		return fmt.Errorf("failed to create apiextensions client: %w", err)
//	}
//	crdInformerFactory := apiextensionsinformer.NewSharedInformerFactory(apiExtClient, 0)
//	crdInformer := crdInformerFactory.Apiextensions().V1().CustomResourceDefinitions().Informer()
//
//	_, err = crdInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
//		AddFunc: func(obj interface{}) {
//			crd := obj.(*apiextensionsv1.CustomResourceDefinition)
//			gvk := schema.GroupVersionKind{
//				Group:   crd.Spec.Group,
//				Version: crd.Spec.Versions[0].Name,
//				Kind:    crd.Spec.Names.Kind,
//			}
//			plural := crd.Spec.Names.Plural
//			r.StartInformerForGVK(ctx, gvk, plural)
//		},
//		DeleteFunc: func(obj interface{}) {
//			crd := obj.(*apiextensionsv1.CustomResourceDefinition)
//			gvk := schema.GroupVersionKind{
//				Group:   crd.Spec.Group,
//				Version: crd.Spec.Versions[0].Name,
//				Kind:    crd.Spec.Names.Kind,
//			}
//			r.StopInformerForGVK(gvk)
//		},
//	})
//	if err != nil {
//		return err
//	}
//	go crdInformerFactory.Start(ctx.Done())
//	return nil
//}
//
//func (r *BackupReconciler) StartInformerForGVK(ctx context.Context, gvk schema.GroupVersionKind, plural string) {
//	key := fmt.Sprintf("%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind)
//
//	r.mu.Lock()
//	if _, exists := r.workers[key]; exists {
//		r.mu.Unlock()
//		return
//	}
//	ch := make(chan backupEvent, 100)
//	r.workers[key] = ch
//	childCtx, cancel := context.WithCancel(ctx)
//	r.workerCancels[key] = cancel
//	r.mu.Unlock()
//	go func() {
//		defer func() {
//			if r := recover(); r != nil {
//				fmt.Printf("worker panic recovered: %v\n", r)
//			}
//		}()
//		r.workerLoop(childCtx, gvk, ch)
//	}()
//	gvr := schema.GroupVersionResource{Group: gvk.Group, Version: gvk.Version, Resource: plural}
//	informer := dynamicinformer.NewFilteredDynamicInformer(
//		r.DynamicClient, gvr, metav1.NamespaceAll, 0, cache.Indexers{}, nil)
//
//	_, err := informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
//		AddFunc: func(obj interface{}) {
//			if u, ok := obj.(*unstructured.Unstructured); ok && u.GetAnnotations()["backup.bastion.io/enabled"] == "true" {
//				r.enqueueOld(childCtx, u)
//			}
//		},
//		UpdateFunc: func(_, newObj interface{}) {
//			if u, ok := newObj.(*unstructured.Unstructured); ok && u.GetAnnotations()["backup.bastion.io/enabled"] == "true" {
//				r.enqueueOld(childCtx, u)
//			}
//		},
//		DeleteFunc: func(obj interface{}) {
//			if u, ok := obj.(*unstructured.Unstructured); ok && u.GetAnnotations()["backup.bastion.io/enabled"] == "true" {
//				r.enqueueOld(childCtx, u)
//			}
//		},
//	})
//	if err != nil {
//		return
//	}
//
//	go informer.Informer().Run(childCtx.Done())
//}
//
//func (r *BackupReconciler) StopInformerForGVK(gvk schema.GroupVersionKind) {
//	key := fmt.Sprintf("%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind)
//
//	r.mu.Lock()
//	defer r.mu.Unlock()
//
//	if cancel, exists := r.workerCancels[key]; exists {
//		cancel() // stop context
//		delete(r.workerCancels, key)
//		delete(r.workers, key) // close worker queue if needed
//		fmt.Printf("stopped informer for %s\n", key)
//	}
//}
//
//func (r *BackupReconciler) enqueueOld(ctx context.Context, obj interface{}) {
//	u, ok := obj.(*unstructured.Unstructured)
//	if !ok {
//		return
//	}
//	gvk := u.GroupVersionKind()
//	key := fmt.Sprintf("%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind)
//	r.mu.Lock()
//	ch, exists := r.workers[key]
//	if !exists {
//		ch = make(chan backupEvent, 100)
//		r.workers[key] = ch
//		go r.workerLoop(ctx, gvk, ch)
//	}
//	r.mu.Unlock()
//	deleted := false
//	if u.GetDeletionTimestamp() != nil && len(u.GetFinalizers()) == 0 {
//		deleted = true
//	}
//	ch <- backupEvent{Object: u.DeepCopy(), Deleted: deleted}
//}
//
//func (r *BackupReconciler) workerLoop(ctx context.Context, gvk schema.GroupVersionKind, ch chan backupEvent) {
//	maxRetries := 3
//	for u := range ch {
//		retryCount := 0
//		for retryCount < maxRetries {
//			dir := filepath.Join(r.BackupRoot, gvk.Group, gvk.Version, gvk.Kind, u.Object.GetNamespace(), u.Object.GetName())
//			store := storage.NewFileSystemWriter(dir)
//			if u.Deleted {
//				err := store.Delete(ctx, gvk, u.Object.GetNamespace(), u.Object.GetName())
//				if err != nil {
//					fmt.Printf("[%s] failed to delete backup: %v (retry %d/%d)\n", gvk.Kind, err, retryCount+1, maxRetries)
//					retryCount++
//					continue
//				}
//			}
//
//			hashStr, err := hash.SanitizeAndHash(u.Object)
//			if err != nil {
//				fmt.Printf("[%s] failed to hash: %v (retry %d/%d)\n", gvk.Kind, err, retryCount+1, maxRetries)
//				retryCount++
//				continue
//			}
//
//			_, oldHash, err := store.Read(ctx, gvk, u.Object.GetNamespace(), u.Object.GetName())
//			if err != nil {
//				fmt.Printf("[%s] failed to read hash: %v (retry %d/%d)\n", gvk.Kind, err, retryCount+1, maxRetries)
//				retryCount++
//				continue
//			}
//			if oldHash == hashStr {
//				break // no change, no need to retry
//			}
//			_, err = store.Write(ctx, u.Object, hashStr)
//			if err != nil {
//				fmt.Printf("[%s] failed to write backup: %v (retry %d/%d)\n", gvk.Kind, err, retryCount+1, maxRetries)
//				retryCount++
//				continue
//			}
//			fmt.Printf("[%s] backup successful for %s/%s\n", gvk.Kind, u.Object.GetNamespace(), u.Object.GetName())
//			break
//		}
//		if retryCount == maxRetries {
//			fmt.Printf("[%s] giving up on backing up %s/%s after %d retries\n", gvk.Kind, u.Object.GetNamespace(), u.Object.GetName(), maxRetries)
//		}
//	}
//}
