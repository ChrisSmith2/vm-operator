/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1

import (
	"context"

	"github.com/golang/glog"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/apis/vmoperator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/rest"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualMachineImage
// +k8s:openapi-gen=true
// +resource:path=virtualmachineimages,strategy=VirtualMachineImageStrategy,rest=VirtualMachineImagesREST
type VirtualMachineImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineImageSpec   `json:"spec,omitempty"`
	Status VirtualMachineImageStatus `json:"status,omitempty"`
}

// VirtualMachineImageSpec defines the desired state of VirtualMachineImage
type VirtualMachineImageSpec struct {
}

// VirtualMachineImageStatus defines the observed state of VirtualMachineImage
type VirtualMachineImageStatus struct {
	Uuid       string `json:"uuid,omitempty"`
	InternalId string `json:"internalId"`
	PowerState string `json:"powerState,omitempty"`
}

// Validate checks that an instance of VirtualMachineImage is well formed
func (VirtualMachineImageStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	image := obj.(*vmoperator.VirtualMachineImage)
	glog.V(4).Infof("Validating fields for VirtualMachineImage %s\n", image.Name)
	errors := field.ErrorList{}
	return errors
}

// DefaultingFunction sets default VirtualMachineImage field values
func (VirtualMachineImageSchemeFns) DefaultingFunction(o interface{}) {
	obj := o.(*VirtualMachineImage)
	// set default field values here
	glog.V(4).Infof("Defaulting fields for VirtualMachineImage %s\n", obj.Name)
}

func NewVirtualMachineImagesREST() rest.Storage {
	return GetRestProvider().ImagesProvider
}
