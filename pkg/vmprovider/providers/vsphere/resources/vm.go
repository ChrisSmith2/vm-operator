/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package resources

import (
	"context"
	"fmt"

	"k8s.io/klog/klogr"

	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"

	"github.com/pkg/errors"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

type VirtualMachine struct {
	Name             string
	vcVirtualMachine *object.VirtualMachine
}

var log = klogr.New().WithName("vmprovider")

// NewVMForCreate returns a VirtualMachine that Create() can be called on
// to create the VM and set the VirtualMachine object reference.
func NewVMForCreate(name string) *VirtualMachine {
	return &VirtualMachine{
		Name: name,
	}
}

func NewVMFromObject(objVm *object.VirtualMachine) (*VirtualMachine, error) {
	return &VirtualMachine{
		Name:             objVm.Name(),
		vcVirtualMachine: objVm,
	}, nil
}

func (vm *VirtualMachine) Create(ctx context.Context, folder *object.Folder, pool *object.ResourcePool, vmSpec *types.VirtualMachineConfigSpec) error {
	if vm.vcVirtualMachine != nil {
		log.Info("Failed to create VM because the VM object is already set", "name", vm.Name)
		return fmt.Errorf("failed to create VM %q because the VM object is already set", vm.Name)
	}

	task, err := folder.CreateVM(ctx, *vmSpec, pool, nil)
	if err != nil {
		return err
	}

	result, err := task.WaitForResult(ctx, nil)
	if err != nil {
		return errors.Wrapf(err, "create VM %q task failed", vm.Name)
	}

	vm.vcVirtualMachine = object.NewVirtualMachine(folder.Client(), result.Result.(types.ManagedObjectReference))

	return nil
}

func (vm *VirtualMachine) Clone(ctx context.Context, folder *object.Folder, cloneSpec *types.VirtualMachineCloneSpec) (*VirtualMachine, error) {
	task, err := vm.vcVirtualMachine.Clone(ctx, folder, cloneSpec.Config.Name, *cloneSpec)
	if err != nil {
		return nil, err
	}

	result, err := task.WaitForResult(ctx, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "clone VM %q task failed", vm.Name)
	}

	clonedObjVm := object.NewVirtualMachine(folder.Client(), result.Result.(types.ManagedObjectReference))
	clonedResVm := VirtualMachine{Name: clonedObjVm.Name(), vcVirtualMachine: clonedObjVm}

	return &clonedResVm, nil
}

func (vm *VirtualMachine) Delete(ctx context.Context) error {

	if vm.vcVirtualMachine == nil {
		return fmt.Errorf("failed to delete VM because the VM object is not set")
	}

	// TODO(bryanv) Move power off if needed call here?

	task, err := vm.vcVirtualMachine.Destroy(ctx)
	if err != nil {
		return err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		return errors.Wrapf(err, "delete VM task failed")
	}

	return nil
}

func (vm *VirtualMachine) Reconfigure(ctx context.Context, configSpec *types.VirtualMachineConfigSpec) error {
	var o mo.VirtualMachine
	log.Info("Reconfiguring VM", "name", vm.Name)
	err := vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), []string{"config"}, &o)
	if err != nil {
		return err
	}

	isDesiredConfig := CompareVmConfig(vm.Name, o.Config, configSpec)
	if isDesiredConfig {
		return nil
	}

	task, err := vm.vcVirtualMachine.Reconfigure(ctx, *configSpec)
	if err != nil {
		return err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		return errors.Wrapf(err, "reconfigure VM %q task failed", vm.Name)
	}

	return nil
}

// IpAddress returns the IpAddress of the VM if powered on, error otherwise
func (vm *VirtualMachine) IpAddress(ctx context.Context) (string, error) {
	var o mo.VirtualMachine

	ps, err := vm.vcVirtualMachine.PowerState(ctx)
	if err != nil || ps == types.VirtualMachinePowerStatePoweredOff {
		return "", err
	}

	// Just get some IP from guest
	err = vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), []string{"guest.ipAddress"}, &o)
	if err != nil {
		return "", err
	}

	if o.Guest == nil {
		log.Info("VM guest info is empty", "name", vm.Name)
		return "", &find.NotFoundError{}
	}

	return o.Guest.IpAddress, nil
}

// CpuAllocation returns the current cpu resource settings from the VM
func (vm *VirtualMachine) CpuAllocation(ctx context.Context) (*types.ResourceAllocationInfo, error) {
	var o mo.VirtualMachine

	err := vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), []string{"config.cpuAllocation"}, &o)
	if err != nil {
		return nil, err
	}

	return o.Config.CpuAllocation, nil
}

// MemoryAllocation returns the current memory resource settings from the VM
func (vm *VirtualMachine) MemoryAllocation(ctx context.Context) (*types.ResourceAllocationInfo, error) {
	var o mo.VirtualMachine

	err := vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), []string{"config.memoryAllocation"}, &o)
	if err != nil {
		return nil, err
	}

	return o.Config.MemoryAllocation, nil
}

func (vm *VirtualMachine) ReferenceValue() string {
	return vm.vcVirtualMachine.Reference().Value
}

func (vm *VirtualMachine) ManagedObject(ctx context.Context) (*mo.VirtualMachine, error) {
	var props mo.VirtualMachine
	if err := vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), nil, &props); err != nil {
		return nil, err
	}
	return &props, nil
}

func (vm *VirtualMachine) ImageFields(ctx context.Context) (powerState, uuid, reference string) {
	ps, _ := vm.vcVirtualMachine.PowerState(ctx)

	powerState = string(ps)
	uuid = vm.vcVirtualMachine.UUID(ctx)
	reference = vm.ReferenceValue()

	return
}

// GetStatus returns a VirtualMachine's Status
func (vm *VirtualMachine) GetStatus(ctx context.Context) (*v1alpha1.VirtualMachineStatus, error) {
	// TODO(bryanv) We should get all the needed fields in one call to VC.

	ps, err := vm.vcVirtualMachine.PowerState(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get PowerState for VirtualMachine: %s", vm.Name)
	}

	host, err := vm.vcVirtualMachine.HostSystem(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get VM HostSystem for VirtualMachine: %s", vm.Name)
	}

	// use ObjectName instead of Name to fetch hostname
	hostname, err := host.ObjectName(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get VM hostname for VirtualMachine: %s", vm.Name)
	}

	ip, err := vm.IpAddress(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get VM IP address for VirtualMachine %s", vm.Name)
	}

	return &v1alpha1.VirtualMachineStatus{
		Host:       hostname,
		Phase:      v1alpha1.Created,
		PowerState: string(ps),
		VmIp:       ip,
		BiosUuid:   vm.vcVirtualMachine.UUID(ctx),
	}, nil
}

func (vm *VirtualMachine) SetPowerState(ctx context.Context, desiredPowerState string) error {

	ps, err := vm.vcVirtualMachine.PowerState(ctx)
	if err != nil {
		log.Error(err, "Failed to get VM power state", "name", vm.Name)
		return err
	}

	log.Info("VM power state", "name", vm.Name, "currentState", ps, "desiredState", desiredPowerState)

	if string(ps) == desiredPowerState {
		return nil
	}

	var task *object.Task

	switch desiredPowerState {
	case v1alpha1.VirtualMachinePoweredOn:
		task, err = vm.vcVirtualMachine.PowerOn(ctx)
	case v1alpha1.VirtualMachinePoweredOff:
		task, err = vm.vcVirtualMachine.PowerOff(ctx)
	default:
		// TODO(bryanv) Suspend? How would we handle reset?
		err = fmt.Errorf("invalid desired power state %s", desiredPowerState)
	}

	if err != nil {
		log.Error(err, "Failed to change VM power state", "name", vm.Name, "desiredState", desiredPowerState)
		return err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		log.Error(err, "VM change power state task failed", "name", vm.Name)
		return err
	}

	return nil
}

// GetVirtualDisks returns the list of VMs vmdks
func (vm *VirtualMachine) GetVirtualDisks(ctx context.Context) (object.VirtualDeviceList, error) {
	deviceList, err := vm.vcVirtualMachine.Device(ctx)
	if err != nil {
		return nil, err
	}

	return deviceList.SelectByType((*types.VirtualDisk)(nil)), nil
}

func (vm *VirtualMachine) GetNetworkDevices(ctx context.Context) ([]types.BaseVirtualDevice, error) {
	devices, err := vm.vcVirtualMachine.Device(ctx)
	if err != nil {
		log.Error(err, "Failed to get devices for VM", "name", vm.Name)
		return nil, err
	}

	return devices.SelectByType((*types.VirtualEthernetCard)(nil)), nil
}

func (vm *VirtualMachine) Customize(ctx context.Context, spec types.CustomizationSpec) error {
	task, err := vm.vcVirtualMachine.Customize(ctx, spec)
	if err != nil {
		log.Error(err, "Failed to customize VM", "name", vm.Name)
		return err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		log.Error(err, "Failed to complete customization for VM", "name", vm.Name)
		return err
	}
	return nil
}

func CompareVmConfig(name string, actualConfig *types.VirtualMachineConfigInfo, desiredConfig *types.VirtualMachineConfigSpec) bool {
	isDesired := true
	if desiredConfig.NumCPUs != actualConfig.Hardware.NumCPU {
		log.Info("Reconfigure VM: NumCPUs", "name", name, "actual", actualConfig.Hardware.NumCPU, "desired", desiredConfig.NumCPUs)
		isDesired = false
	}

	if desiredConfig.MemoryMB != int64(actualConfig.Hardware.MemoryMB) {
		log.Info("Reconfigure VM: MemoryMB", "name", name, "actual", actualConfig.Hardware.MemoryMB, "desired", desiredConfig.MemoryMB)
		isDesired = false
	}

	if (desiredConfig.CpuAllocation != nil) && (desiredConfig.CpuAllocation.Reservation != nil) && (*actualConfig.CpuAllocation.Reservation != *desiredConfig.CpuAllocation.Reservation) {
		log.Info("Reconfigure VM: CpuAllocation Reservation", "name", name, "actual", *actualConfig.CpuAllocation.Reservation, "desired", *desiredConfig.CpuAllocation.Reservation)
		isDesired = false
	}

	if (desiredConfig.CpuAllocation != nil) && (desiredConfig.CpuAllocation.Limit != nil) && (*actualConfig.CpuAllocation.Limit != *desiredConfig.CpuAllocation.Limit) {
		log.Info("Reconfigure VM: CpuAllocation Limit", "actual", *actualConfig.CpuAllocation.Limit, "desired", *desiredConfig.CpuAllocation.Limit)
		isDesired = false
	}

	if (desiredConfig.MemoryAllocation != nil) && (desiredConfig.MemoryAllocation.Reservation != nil) && (*actualConfig.MemoryAllocation.Reservation != *desiredConfig.MemoryAllocation.Reservation) {
		log.Info("Reconfigure VM: MemoryAllocation Reservation", "name", name, "actual", *actualConfig.MemoryAllocation.Reservation, "desired", *desiredConfig.MemoryAllocation.Reservation)
		isDesired = false
	}

	if (desiredConfig.MemoryAllocation != nil) && (desiredConfig.MemoryAllocation.Limit != nil) && (*actualConfig.MemoryAllocation.Limit != *desiredConfig.MemoryAllocation.Limit) {
		log.Info("Reconfigure VM: MemoryAllocation Limit", "name", name, "actual", *actualConfig.MemoryAllocation.Limit, "desired", *desiredConfig.MemoryAllocation.Limit)
		isDesired = false
	}

	return isDesired
}
