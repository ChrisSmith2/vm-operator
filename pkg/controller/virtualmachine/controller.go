/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachine

import (
	"context"
	"time"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/builders"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"vmware.com/kubevsphere/pkg"
	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
	clientSet "vmware.com/kubevsphere/pkg/client/clientset_generated/clientset"
	vmclientSet "vmware.com/kubevsphere/pkg/client/clientset_generated/clientset/typed/vmoperator/v1alpha1"
	listers "vmware.com/kubevsphere/pkg/client/listers_generated/vmoperator/v1alpha1"
	"vmware.com/kubevsphere/pkg/controller/sharedinformers"
	vmprov "vmware.com/kubevsphere/pkg/vmprovider"
	"vmware.com/kubevsphere/pkg/vmprovider/iface"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere"
)

// +controller:group=vmoperator,version=v1alpha1,kind=VirtualMachine,resource=virtualmachines
type VirtualMachineControllerImpl struct {
	builders.DefaultControllerFns

	informers *sharedinformers.SharedInformers

	vmServiceLister listers.VirtualMachineServiceLister

	// lister indexes properties about VirtualMachine
	vmLister listers.VirtualMachineLister

	clientSet   clientSet.Interface
	vmClientSet vmclientSet.VirtualMachineInterface

	vmProvider iface.VirtualMachineProviderInterface
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *VirtualMachineControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {

	c.informers = arguments.GetSharedInformers()

	vmOperator := arguments.GetSharedInformers().Factory.Vmoperator().V1alpha1()
	// Use the lister for indexing virtualmachines labels
	c.vmLister = vmOperator.VirtualMachines().Lister()
	c.vmServiceLister = vmOperator.VirtualMachineServices().Lister()

	clientSet, err := clientSet.NewForConfig(arguments.GetRestConfig())
	if err != nil {
		glog.Fatalf("Failed to create the virtual machine client: %v", err)
	}
	c.clientSet = clientSet

	c.vmClientSet = clientSet.VmoperatorV1alpha1().VirtualMachines(corev1.NamespaceDefault)

	vsphere.InitProvider(arguments.GetSharedInformers().KubernetesClientSet)

	// Get a vmprovider instance
	vmProvider, err := vmprov.NewVmProvider()
	if err != nil {
		glog.Fatalf("Failed to find vmprovider: %s", err)
	}
	c.vmProvider = vmProvider

}

// Function to filter a string from a list. Returns the filtered list
func (c *VirtualMachineControllerImpl) filter(list []string, strToFilter string) (newList []string) {
	for _, item := range list {
		if item != strToFilter {
			newList = append(newList, item)
		}
	}
	return
}

// Function to determine if a list contains s specific string
func (c *VirtualMachineControllerImpl) contains(list []string, strToSearch string) bool {
	for _, item := range list {
		if item == strToSearch {
			return true
		}
	}
	return false
}

func (c *VirtualMachineControllerImpl) postVmServiceEventsToWorkqueue(vm *v1alpha1.VirtualMachine) error {

	glog.V(4).Infof("VM update: %v", vm.Name)

	vmServices, err := c.vmServiceLister.VirtualMachineServices(vm.Namespace).List(labels.Everything())
	if err != nil {
		return err
	}

	for _, vmService := range vmServices {
		key, err := cache.MetaNamespaceKeyFunc(vmService)
		if err == nil {
			c.informers.WorkerQueues["VirtualMachineService"].Queue.Add(key)
		}
	}

	return nil
}

// Reconcile handles enqueued messages
func (c *VirtualMachineControllerImpl) Reconcile(vmToReconcile *v1alpha1.VirtualMachine) error {
	glog.V(0).Infof("Running reconcile VirtualMachine for %s\n", vmToReconcile.Name)

	startTime := time.Now()
	defer func() {
		glog.V(0).Infof("Finished syncing vm %q (%v)", vmToReconcile.Name, time.Since(startTime))
	}()

	// Trigger vmservice evaluation
	err := c.postVmServiceEventsToWorkqueue(vmToReconcile)

	// We hold a Finalizer on the VM, so it must be present
	if !vmToReconcile.ObjectMeta.DeletionTimestamp.IsZero() {
		// This VM has been deleted, sync with backend
		glog.Infof("Deletion timestamp is non-zero")

		// Noop if our finalizer is not present
		//if u.ObjectMeta.Finalizers()
		if !c.contains(vmToReconcile.ObjectMeta.Finalizers, v1alpha1.VirtualMachineFinalizer) {
			glog.Infof("reconciling virtual machine object %v causes a no-op as there is no finalizer.", vmToReconcile.Name)
			return nil
		}

		glog.Infof("reconciling virtual machine object %v triggers delete.", vmToReconcile.Name)
		if err := c.processVmDeletion(vmToReconcile); err != nil {
			glog.Errorf("Error deleting machine object %v; %v", vmToReconcile.Name, err)
			return err
		}

		// Remove finalizer on successful deletion.
		glog.Infof("virtual machine object %v deletion successful, removing finalizer.", vmToReconcile.Name)
		vmToReconcile.ObjectMeta.Finalizers = c.filter(vmToReconcile.ObjectMeta.Finalizers, v1alpha1.VirtualMachineFinalizer)
		if _, err := c.vmClientSet.Update(vmToReconcile); err != nil {
			glog.Errorf("Error removing finalizer from machine object %v; %v", vmToReconcile.Name, err)
			return err
		}
		return nil
	}

	// vm holds the latest vm info from apiserver
	vm, err := c.vmLister.VirtualMachines(vmToReconcile.Namespace).Get(vmToReconcile.Name)
	if err != nil {
		glog.Infof("Unable to retrieve vm %v from store: %v", vmToReconcile.Name, err)
		return err
	}

	_, err = c.processVmCreateOrUpdate(vm)
	if err != nil {
		glog.Infof("Failed to process Create or Update for %s: %s", vmToReconcile.Name, err)
		return err
	}

	return err
}

func (c *VirtualMachineControllerImpl) processVmDeletion(vmToDelete *v1alpha1.VirtualMachine) error {
	glog.Infof("Process VM Deletion for vm %s", vmToDelete.Name)

	vmsProvider, supported := c.vmProvider.VirtualMachines()
	if !supported {
		glog.Errorf("Provider doesn't support vms func")
		return errors.NewMethodNotSupported(schema.GroupResource{Group: "vmoperator", Resource: "VirtualMachines"}, "list")
	}

	ctx := context.TODO()

	err := vmsProvider.DeleteVirtualMachine(ctx, vmToDelete)
	if err != nil {
		if errors.IsNotFound(err) {
			glog.Infof("Failed to delete vm %s, already deleted?", vmToDelete.Name)
		} else {
			glog.Errorf("Failed to delete vm %s: %s", vmToDelete.Name, err)
			return err
		}
	}

	glog.V(4).Infof("Deleted VM %s", vmToDelete.Name)
	return nil
}

// Process a level trigger for this VM.  Process a create if the VM doesn't exist.  Process an Update to the VM if
// it is already present
func (c *VirtualMachineControllerImpl) processVmCreateOrUpdate(vmToUpdate *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	glog.Infof("Process VM Create or Update for vm %s", vmToUpdate.Name)

	vmsProvider, supported := c.vmProvider.VirtualMachines()
	if !supported {
		glog.Errorf("Provider doesn't support vms func")
		return nil, errors.NewMethodNotSupported(schema.GroupResource{Group: "vmoperator", Resource: "VirtualMachines"}, "list")
	}

	ctx := context.TODO()
	vm, err := vmsProvider.GetVirtualMachine(ctx, vmToUpdate.Name)
	var newVm *v1alpha1.VirtualMachine
	switch {
	case errors.IsNotFound(err):
		glog.Infof("VM Lookup Error is %s", err)
		glog.Infof("VM doesn't exist in backend provider.  Creating now")
		newVm, err = c.processVmCreate(ctx, vmsProvider, vmToUpdate)
	case err != nil:
		glog.Infof("Unable to retrieve vm %v from store: %v", vmToUpdate.Name, err)
	default:
		//glog.V(4).Infof("Acquired VM %s %s", vm.Name, vm.Status.ConfigStatus.InternalId)
		glog.Infof("Updating Vm %s", vm.Name)
		newVm, err = c.processVmUpdate(ctx, vmsProvider, vmToUpdate)
	}

	if err != nil {
		glog.Errorf("Failed to create or update VM in provider %s: %s", vmToUpdate.Name, err)
		return nil, err
	}

	// Update object
	_, err = c.vmClientSet.UpdateStatus(newVm)
	if err != nil {
		glog.Errorf("Failed to update VM Resource in Storage %s: %s", newVm.Name, err)
	}
	return newVm, err
}

// Process a create event for a new VM.
func (c *VirtualMachineControllerImpl) processVmCreate(ctx context.Context, vmsProvider iface.VirtualMachines, vmToCreate *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	glog.Infof("Creating VM: %s", vmToCreate.Name)
	newVm, err := vmsProvider.CreateVirtualMachine(ctx, vmToCreate)
	if err != nil {
		glog.Errorf("Provider Failed to Create VM %s: %s", vmToCreate.Name, err)
		return nil, err
	}

	pkg.AddAnnotations(&newVm.ObjectMeta)
	return newVm, err
}

// Process an update event for an existing VM.
func (c *VirtualMachineControllerImpl) processVmUpdate(ctx context.Context, vmsProvider iface.VirtualMachines, vmToUpdate *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	glog.Infof("Updating VM: %s", vmToUpdate.Name)
	newVm, err := vmsProvider.UpdateVirtualMachine(ctx, vmToUpdate)
	if err != nil {
		glog.Errorf("Provider Failed to Update VM %s: %s", vmToUpdate.Name, err)
	}

	return newVm, err
}

func (c *VirtualMachineControllerImpl) Get(namespace, name string) (*v1alpha1.VirtualMachine, error) {
	return c.vmLister.VirtualMachines(namespace).Get(name)
}
