// Copyright (c) 2018-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmprovider

import (
	"context"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

type VMMetadata struct {
	Data      map[string]string
	Transport v1alpha1.VirtualMachineMetadataTransport
}

type VMConfigArgs struct {
	VMClass            v1alpha1.VirtualMachineClass
	VMImage            *v1alpha1.VirtualMachineImage
	ResourcePolicy     *v1alpha1.VirtualMachineSetResourcePolicy
	VMMetadata         VMMetadata
	StorageProfileID   string
	ContentLibraryUUID string
}

// VirtualMachineProviderInterface is a plugable interface for VM Providers.
type VirtualMachineProviderInterface interface {
	Name() string

	// Initialize provides the cloud with a kubernetes client builder and may spawn goroutines
	// to perform housekeeping or run custom controllers specific to the cloud provider.
	// Any tasks started here should be cleaned up when the stop channel closes.
	Initialize(stop <-chan struct{})

	DoesVirtualMachineExist(ctx context.Context, vm *v1alpha1.VirtualMachine) (bool, error)
	PlaceVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs VMConfigArgs) error
	CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs VMConfigArgs) error
	UpdateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs VMConfigArgs) error
	DeleteVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) error
	GetVirtualMachineGuestHeartbeat(ctx context.Context, vm *v1alpha1.VirtualMachine) (v1alpha1.GuestHeartbeatStatus, error)
	GetVirtualMachineWebMKSTicket(ctx context.Context, vm *v1alpha1.VirtualMachine, pubKey string) (string, error)

	CreateOrUpdateVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error
	IsVirtualMachineSetResourcePolicyReady(ctx context.Context, availabilityZoneName string, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error)
	DeleteVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error

	// "Infra" related
	UpdateVcPNID(ctx context.Context, vcPNID, vcPort string) error
	ClearSessionsAndClient(ctx context.Context)
	DeleteNamespaceSessionInCache(ctx context.Context, namespace string) error
	ComputeClusterCPUMinFrequency(ctx context.Context) error

	ListItemsFromContentLibrary(ctx context.Context, contentLibrary *v1alpha1.ContentLibraryProvider) ([]string, error)
	GetVirtualMachineImageFromContentLibrary(ctx context.Context, contentLibrary *v1alpha1.ContentLibraryProvider, itemID string,
		currentCLImages map[string]v1alpha1.VirtualMachineImage) (*v1alpha1.VirtualMachineImage, error)
}
