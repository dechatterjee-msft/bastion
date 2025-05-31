package controllers

import (
	"context"
	"github.com/bastion/test/utils"
	"os"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"strings"
	"testing"
	"time"

	"github.com/bastion/internal/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	testEnv    *envtest.Environment
	k8sClient  client.Client
	backupRoot string
	stopFunc   context.CancelFunc
)

func TestBackup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Backup Controller BDD Suite")
}

var _ = BeforeSuite(func() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	SetDefaultEventuallyTimeout(20 * time.Second)
	SetDefaultEventuallyPollingInterval(1 * time.Second)

	ctx, cancel := context.WithCancel(context.TODO())
	stopFunc = cancel

	backupRoot = os.TempDir()

	By("Starting test environment")
	testEnv = &envtest.Environment{}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())
	// 🚨 Dynamically install CRDs
	By("Installing CRDs into envtest")
	Expect(utils.InstallCRDs(ctx, cfg, filepath.Join("..", "..", "test", "data"))).To(Succeed())
	By("Starting manager and backup controller")
	mgr, err := manager.New(cfg, manager.Options{})
	Expect(err).NotTo(HaveOccurred())
	k8sClient = mgr.GetClient()
	bkpCtrl := NewBackupController(&config.Options{
		BackupRoot: backupRoot,
		MaxRetries: 5,
		GcRetain:   5 * time.Minute,
	})
	Expect(bkpCtrl.Setup(ctx, mgr)).To(Succeed())
	go func() {
		Expect(mgr.Start(ctx)).To(Succeed())
	}()
})

var _ = AfterSuite(func() {
	stopFunc()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = DescribeTable("Backup Controller",
	func(kind string, spec map[string]interface{}) {
		ctx := context.Background()
		name := "test-" + strings.ToLower(kind)

		cr := &unstructured.Unstructured{}
		cr.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "demo.bastion.io",
			Version: "v1",
			Kind:    kind,
		})
		cr.SetName(name)
		cr.SetNamespace("default")
		cr.SetLabels(map[string]string{
			"backup.bastion.io/enabled": "true",
		})
		cr.Object["spec"] = spec

		By("Creating CR")
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())

		time.Sleep(3 * time.Second)

		manifest := filepath.Join(backupRoot, "demo.bastion.io", "v1", kind, cr.GetNamespace(), cr.GetName(), "manifest.yaml")
		hash := filepath.Join(backupRoot, "demo.bastion.io", "v1", kind, cr.GetNamespace(), cr.GetName(), "hash.txt")

		By("Expecting backup files to exist")
		Expect(manifest).Should(BeAnExistingFile())
		Expect(hash).Should(BeAnExistingFile())

		By("Updating CR")
		patch := client.MergeFrom(cr.DeepCopy())
		cr.Object["spec"].(map[string]interface{})["extra"] = "value"
		Expect(k8sClient.Patch(ctx, cr, patch)).To(Succeed())

		time.Sleep(3 * time.Second)

		By("Expecting backup updated")
		Expect(manifest).Should(BeAnExistingFile())

		By("Deleting CR")
		Expect(k8sClient.Delete(ctx, cr)).To(Succeed())

		time.Sleep(5 * time.Second)

		tombstone := filepath.Join(backupRoot, "demo.bastion.io", "v1", kind, cr.GetNamespace(), cr.GetName(), "tombstone")
		Expect(tombstone).Should(BeAnExistingFile())
	},

	Entry("Task CR", "Task", map[string]interface{}{
		"description": "Sample Task",
	}),
	Entry("Workflow CR", "Workflow", map[string]interface{}{
		"steps": []string{"build", "test", "deploy"},
	}),
	Entry("Pipeline CR", "Pipeline", map[string]interface{}{
		"stages": []string{"dev", "qa", "prod"},
	}),
)
