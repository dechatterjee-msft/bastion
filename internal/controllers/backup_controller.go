package controllers

import (
	"context"
	"fmt"

	"github.com/bastion/internal/config"
	"github.com/bastion/internal/dispatcher"
	"github.com/bastion/internal/hash"
	"github.com/bastion/internal/storage"
	"github.com/bastion/internal/worker"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsinformer "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type BackupController struct {
	Dispatcher      *dispatcher.Dispatcher
	Hasher          hash.Hasher
	StoreFactory    func(base string) storage.StorageWriter
	InformerFactory dynamicinformer.DynamicSharedInformerFactory
	MaxRetries      int
	BaseDir         string
}

func NewBackupController(cfg *config.Options) *BackupController {
	return &BackupController{
		Dispatcher:   dispatcher.NewDispatcher(),
		Hasher:       hash.NewDefaultHasher(),
		StoreFactory: func(base string) storage.StorageWriter { return storage.NewFileSystemWriter(base) },
		MaxRetries:   cfg.MaxRetries,
		BaseDir:      cfg.BackupRoot,
	}
}

func (bc *BackupController) Setup(ctx context.Context, mgr manager.Manager) error {
	dynamicClient := dynamic.NewForConfigOrDie(mgr.GetConfig())
	bc.InformerFactory = dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, 0)

	apiExtClient, err := apiextensionsclientset.NewForConfig(mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("failed to create apiextensions client: %w", err)
	}

	crdInformerFactory := apiextensionsinformer.NewSharedInformerFactory(apiExtClient, 0)
	crdInformer := crdInformerFactory.Apiextensions().V1().CustomResourceDefinitions().Informer()

	_, err = crdInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			crd := obj.(*apiextensionsv1.CustomResourceDefinition)
			gvk := schema.GroupVersionKind{
				Group:   crd.Spec.Group,
				Version: crd.Spec.Versions[0].Name,
				Kind:    crd.Spec.Names.Kind,
			}
			gvr := schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  crd.Spec.Versions[0].Name,
				Resource: crd.Spec.Names.Plural,
			}

			bw := worker.NewBackupWorker(gvk, bc.Hasher, bc.StoreFactory(bc.BaseDir), 100, bc.MaxRetries)
			_ = bc.Dispatcher.Register(ctx, gvr, gvk, bc.InformerFactory, bw)
		},
		DeleteFunc: func(obj interface{}) {
			crd := obj.(*apiextensionsv1.CustomResourceDefinition)
			gvk := schema.GroupVersionKind{
				Group:   crd.Spec.Group,
				Version: crd.Spec.Versions[0].Name,
				Kind:    crd.Spec.Names.Kind,
			}
			_ = bc.Dispatcher.Stop(gvk)
		},
	})
	if err != nil {
		return err
	}

	go crdInformerFactory.Start(ctx.Done())
	return nil
}
