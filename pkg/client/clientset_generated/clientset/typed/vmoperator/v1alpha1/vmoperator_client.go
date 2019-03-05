/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	rest "k8s.io/client-go/rest"
	v1alpha1 "vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
	"vmware.com/kubevsphere/pkg/client/clientset_generated/clientset/scheme"
)

type VmoperatorV1alpha1Interface interface {
	RESTClient() rest.Interface
	VirtualMachinesGetter
	VirtualMachineClassesGetter
	VirtualMachineImagesGetter
	VirtualMachineServicesGetter
}

// VmoperatorV1alpha1Client is used to interact with features provided by the vmoperator.vmware.com group.
type VmoperatorV1alpha1Client struct {
	restClient rest.Interface
}

func (c *VmoperatorV1alpha1Client) VirtualMachines(namespace string) VirtualMachineInterface {
	return newVirtualMachines(c, namespace)
}

func (c *VmoperatorV1alpha1Client) VirtualMachineClasses(namespace string) VirtualMachineClassInterface {
	return newVirtualMachineClasses(c, namespace)
}

func (c *VmoperatorV1alpha1Client) VirtualMachineImages(namespace string) VirtualMachineImageInterface {
	return newVirtualMachineImages(c, namespace)
}

func (c *VmoperatorV1alpha1Client) VirtualMachineServices(namespace string) VirtualMachineServiceInterface {
	return newVirtualMachineServices(c, namespace)
}

// NewForConfig creates a new VmoperatorV1alpha1Client for the given config.
func NewForConfig(c *rest.Config) (*VmoperatorV1alpha1Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &VmoperatorV1alpha1Client{client}, nil
}

// NewForConfigOrDie creates a new VmoperatorV1alpha1Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *VmoperatorV1alpha1Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new VmoperatorV1alpha1Client for the given RESTClient.
func New(c rest.Interface) *VmoperatorV1alpha1Client {
	return &VmoperatorV1alpha1Client{c}
}

func setConfigDefaults(config *rest.Config) error {
	gv := v1alpha1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: scheme.Codecs}

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *VmoperatorV1alpha1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}
