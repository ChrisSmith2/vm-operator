// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"fmt"
	"net"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha2"
)

func Convert_v1alpha1_VirtualMachineVolume_To_v1alpha2_VirtualMachineVolume(
	in *VirtualMachineVolume, out *v1alpha2.VirtualMachineVolume, s apiconversion.Scope) error {

	// TODO: v1a2 needs InstanceVolumeClaim.
	if claim := in.PersistentVolumeClaim; claim != nil {
		src := claim.PersistentVolumeClaimVolumeSource
		out.PersistentVolumeClaim = &src
	}

	// TODO: in.VsphereVolume

	return autoConvert_v1alpha1_VirtualMachineVolume_To_v1alpha2_VirtualMachineVolume(in, out, s)
}

func Convert_v1alpha2_VirtualMachineVolume_To_v1alpha1_VirtualMachineVolume(
	in *v1alpha2.VirtualMachineVolume, out *VirtualMachineVolume, s apiconversion.Scope) error {

	if claim := in.PersistentVolumeClaim; claim != nil {
		out.PersistentVolumeClaim = &PersistentVolumeClaimVolumeSource{
			PersistentVolumeClaimVolumeSource: *claim,
		}
	}

	return autoConvert_v1alpha2_VirtualMachineVolume_To_v1alpha1_VirtualMachineVolume(in, out, s)
}

func Convert_v1alpha1_VirtualMachineVolumeProvisioningOptions_To_v1alpha2_VirtualMachineVolumeProvisioningOptions(
	in *VirtualMachineVolumeProvisioningOptions, out *v1alpha2.VirtualMachineVolumeProvisioningOptions, s apiconversion.Scope) error {

	in.ThinProvisioned = out.ThinProvision
	in.EagerZeroed = out.EagerZero

	return autoConvert_v1alpha1_VirtualMachineVolumeProvisioningOptions_To_v1alpha2_VirtualMachineVolumeProvisioningOptions(in, out, s)
}

func Convert_v1alpha2_VirtualMachineVolumeProvisioningOptions_To_v1alpha1_VirtualMachineVolumeProvisioningOptions(
	in *v1alpha2.VirtualMachineVolumeProvisioningOptions, out *VirtualMachineVolumeProvisioningOptions, s apiconversion.Scope) error {

	out.ThinProvisioned = in.ThinProvision
	out.EagerZeroed = in.EagerZero

	return autoConvert_v1alpha2_VirtualMachineVolumeProvisioningOptions_To_v1alpha1_VirtualMachineVolumeProvisioningOptions(in, out, s)
}

func convert_v1alpha1_VmMetadata_To_v1alpha2_BootstrapSpec(
	in *VirtualMachineMetadata) v1alpha2.VirtualMachineBootstrapSpec {

	out := v1alpha2.VirtualMachineBootstrapSpec{}

	if in != nil {
		objectName := in.SecretName
		if objectName == "" {
			objectName = in.ConfigMapName
		}

		switch in.Transport {
		case VirtualMachineMetadataExtraConfigTransport:
			out.CloudInit = &v1alpha2.VirtualMachineBootstrapCloudInitSpec{
				RawCloudConfig: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: objectName},
					Key:                  "guestinfo.userdata",
				},
			}
		case VirtualMachineMetadataOvfEnvTransport:
			// TODO: Assume LinuxPrep+VAppConfig for now but can we infer when to use CloudInit here?
			out.LinuxPrep = &v1alpha2.VirtualMachineBootstrapLinuxPrepSpec{}
			out.VAppConfig = &v1alpha2.VirtualMachineBootstrapVAppConfigSpec{
				RawProperties: objectName,
			}
			/*
				out.CloudInit = &v1alpha2.VirtualMachineBootstrapCloudInitSpec{
					RawCloudConfig: corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: objectName},
					},
				}
			*/
		case VirtualMachineMetadataVAppConfigTransport:
			out.VAppConfig = &v1alpha2.VirtualMachineBootstrapVAppConfigSpec{
				RawProperties: objectName,
			}
		case VirtualMachineMetadataCloudInitTransport:
			out.CloudInit = &v1alpha2.VirtualMachineBootstrapCloudInitSpec{
				RawCloudConfig: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: objectName},
					Key:                  "user-data",
				},
			}
		case VirtualMachineMetadataSysprepTransport:
			out.Sysprep = &v1alpha2.VirtualMachineBootstrapSysprepSpec{
				RawSysprep: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: objectName},
					Key:                  "unattend",
				},
			}
		}
	}

	return out
}

func convert_v1alpha2_BootstrapSpec_To_v1alpha1_VmMetadata(
	in v1alpha2.VirtualMachineBootstrapSpec) *VirtualMachineMetadata {

	if apiequality.Semantic.DeepEqual(in, v1alpha2.VirtualMachineBootstrapSpec{}) {
		return nil
	}

	out := &VirtualMachineMetadata{}

	if cloudInit := in.CloudInit; cloudInit != nil {
		// TODO: Here we don't know if this was originally a Secret or a ConfigMap.
		out.SecretName = cloudInit.RawCloudConfig.Name

		switch cloudInit.RawCloudConfig.Key {
		case "guestinfo.userdata":
			out.Transport = VirtualMachineMetadataExtraConfigTransport
		case "user-data":
			out.Transport = VirtualMachineMetadataCloudInitTransport
		}
	} else if in.VAppConfig != nil {
		out.SecretName = in.VAppConfig.RawProperties

		if in.LinuxPrep != nil {
			out.Transport = VirtualMachineMetadataOvfEnvTransport
		} else {
			out.Transport = VirtualMachineMetadataVAppConfigTransport
		}
	}

	return out
}

func convert_v1alpha1_NetworkInterface_To_v1alpha2_NetworkInterfaceSpec(
	idx int, in VirtualMachineNetworkInterface) v1alpha2.VirtualMachineNetworkInterfaceSpec {

	out := v1alpha2.VirtualMachineNetworkInterfaceSpec{}
	out.Name = fmt.Sprintf("eth%d", idx)
	out.Network.Name = in.NetworkName

	switch in.NetworkType {
	case "vsphere-distributed":
		out.Network.TypeMeta.APIVersion = "netoperator.vmware.com/v1alpha1"
		out.Network.TypeMeta.Kind = "Network"
	case "nsx-t":
		out.Network.TypeMeta.APIVersion = "vmware.com/v1alpha1"
		out.Network.TypeMeta.Kind = "VirtualNetwork"
	}

	return out
}

func convert_v1alpha2_NetworkInterfaceSpec_To_v1alpha1_NetworkInterface(
	in v1alpha2.VirtualMachineNetworkInterfaceSpec) VirtualMachineNetworkInterface {

	out := VirtualMachineNetworkInterface{
		NetworkName: in.Network.Name,
	}

	switch in.Network.TypeMeta.Kind {
	case "Network":
		out.NetworkType = "vsphere-distributed"
	case "VirtualNetwork":
		out.NetworkType = "nsx-t"
	}

	return out
}

func convert_v1alpha1_Probe_To_v1alpha2_ReadinessProbeSpec(in *Probe) v1alpha2.VirtualMachineReadinessProbeSpec {
	out := v1alpha2.VirtualMachineReadinessProbeSpec{}

	if in != nil {
		out.TimeoutSeconds = in.TimeoutSeconds
		out.PeriodSeconds = in.PeriodSeconds

		if in.TCPSocket != nil {
			out.TCPSocket = &v1alpha2.TCPSocketAction{
				Port: in.TCPSocket.Port,
				Host: in.TCPSocket.Host,
			}
		}

		if in.GuestHeartbeat != nil {
			out.GuestHeartbeat = &v1alpha2.GuestHeartbeatAction{
				ThresholdStatus: v1alpha2.GuestHeartbeatStatus(in.GuestHeartbeat.ThresholdStatus),
			}
		}

		// out.GuestInfo =
	}

	return out
}

func convert_v1alpha2_ReadinessProbeSpec_To_v1alpha1_Probe(in v1alpha2.VirtualMachineReadinessProbeSpec) *Probe {

	if apiequality.Semantic.DeepEqual(in, v1alpha2.VirtualMachineReadinessProbeSpec{}) {
		return nil
	}

	out := &Probe{
		TimeoutSeconds: in.TimeoutSeconds,
		PeriodSeconds:  in.PeriodSeconds,
	}

	if in.TCPSocket != nil {
		out.TCPSocket = &TCPSocketAction{
			Port: in.TCPSocket.Port,
			Host: in.TCPSocket.Host,
		}
	}

	if in.GuestHeartbeat != nil {
		out.GuestHeartbeat = &GuestHeartbeatAction{
			ThresholdStatus: GuestHeartbeatStatus(in.GuestHeartbeat.ThresholdStatus),
		}
	}

	// = in.GuestInfo

	return out
}

func convert_v1alpha1_VirtualMachineAdvancedOptions_To_v1alpha2_VirtualMachineAdvancedSpec(
	in *VirtualMachineAdvancedOptions) v1alpha2.VirtualMachineAdvancedSpec {

	out := v1alpha2.VirtualMachineAdvancedSpec{}

	if in != nil {
		// out.BootDiskCapacity =

		if opts := in.DefaultVolumeProvisioningOptions; opts != nil {
			// opts.ThinProvisioned
			// opts.EagerZeroed
			out.DefaultVolumeProvisioningMode = "" // TODO: Define enum values
		}

		if in.ChangeBlockTracking != nil {
			out.ChangeBlockTracking = *in.ChangeBlockTracking
		}
	}

	return out
}

func convert_v1alpha2_VirtualMachineAdvancedSpec_To_v1alpha1_VirtualMachineAdvancedOptions(
	in v1alpha2.VirtualMachineAdvancedSpec) *VirtualMachineAdvancedOptions {

	out := &VirtualMachineAdvancedOptions{}

	if in.ChangeBlockTracking {
		out.ChangeBlockTracking = pointer.Bool(true)
	}

	if in.DefaultVolumeProvisioningMode != "" {
		// TODO: Need ProvisioningMode enums
		out.DefaultVolumeProvisioningOptions = &VirtualMachineVolumeProvisioningOptions{}
	}

	return out
}

func convert_v1alpha1_Network_To_v1alpha2_NetworkStatus(
	vmIP string, in []NetworkInterfaceStatus) *v1alpha2.VirtualMachineNetworkStatus {

	if vmIP == "" && len(in) == 0 {
		return nil
	}

	out := &v1alpha2.VirtualMachineNetworkStatus{}

	if net.ParseIP(vmIP).To4() != nil {
		out.PrimaryIP4 = vmIP
	} else {
		out.PrimaryIP6 = vmIP
	}

	ipAddrsToAddrStatus := func(ipAddr []string) []v1alpha2.VirtualMachineNetworkInterfaceIPAddrStatus {
		statuses := make([]v1alpha2.VirtualMachineNetworkInterfaceIPAddrStatus, 0, len(ipAddr))
		for _, ip := range ipAddr {
			statuses = append(statuses, v1alpha2.VirtualMachineNetworkInterfaceIPAddrStatus{Address: ip})
		}
		return statuses
	}

	for _, inI := range in {
		interfaceStatus := v1alpha2.VirtualMachineNetworkInterfaceStatus{
			IP: v1alpha2.VirtualMachineNetworkInterfaceIPStatus{
				Addresses: ipAddrsToAddrStatus(inI.IpAddresses),
				MACAddr:   inI.MacAddress,
			},
		}
		out.Interfaces = append(out.Interfaces, interfaceStatus)
	}

	return out
}

func convert_v1alpha2_NetworkStatus_To_v1alpha1_Network(
	in *v1alpha2.VirtualMachineNetworkStatus) (string, []NetworkInterfaceStatus) {

	if in == nil {
		return "", nil
	}

	vmIP := in.PrimaryIP4
	if vmIP == "" {
		vmIP = in.PrimaryIP6
	}

	addrStatusToIPAddrs := func(addrStatus []v1alpha2.VirtualMachineNetworkInterfaceIPAddrStatus) []string {
		ipAddrs := make([]string, 0, len(addrStatus))
		for _, a := range addrStatus {
			ipAddrs = append(ipAddrs, a.Address)
		}
		return ipAddrs
	}

	out := make([]NetworkInterfaceStatus, 0, len(in.Interfaces))
	for _, i := range in.Interfaces {
		interfaceStatus := NetworkInterfaceStatus{
			Connected:   true,
			MacAddress:  i.IP.MACAddr,
			IpAddresses: addrStatusToIPAddrs(i.IP.Addresses),
		}
		out = append(out, interfaceStatus)
	}

	return vmIP, out
}

func Convert_v1alpha1_VirtualMachineSpec_To_v1alpha2_VirtualMachineSpec(
	in *VirtualMachineSpec, out *v1alpha2.VirtualMachineSpec, s apiconversion.Scope) error {

	out.Bootstrap = convert_v1alpha1_VmMetadata_To_v1alpha2_BootstrapSpec(in.VmMetadata)

	for i, networkInterface := range in.NetworkInterfaces {
		networkInterfaceSpec := convert_v1alpha1_NetworkInterface_To_v1alpha2_NetworkInterfaceSpec(i, networkInterface)
		out.Network.Interfaces = append(out.Network.Interfaces, networkInterfaceSpec)
	}
	// TODO: out.Network.Network = ???

	out.ReadinessProbe = convert_v1alpha1_Probe_To_v1alpha2_ReadinessProbeSpec(in.ReadinessProbe)
	out.Advanced = convert_v1alpha1_VirtualMachineAdvancedOptions_To_v1alpha2_VirtualMachineAdvancedSpec(in.AdvancedOptions)
	out.Reserved.ResourcePolicyName = in.ResourcePolicyName

	// Deprecated:
	// in.Ports

	return autoConvert_v1alpha1_VirtualMachineSpec_To_v1alpha2_VirtualMachineSpec(in, out, s)
}

func Convert_v1alpha2_VirtualMachineSpec_To_v1alpha1_VirtualMachineSpec(
	in *v1alpha2.VirtualMachineSpec, out *VirtualMachineSpec, s apiconversion.Scope) error {

	out.VmMetadata = convert_v1alpha2_BootstrapSpec_To_v1alpha1_VmMetadata(in.Bootstrap)

	for _, networkInterfaceSpec := range in.Network.Interfaces {
		networkInterface := convert_v1alpha2_NetworkInterfaceSpec_To_v1alpha1_NetworkInterface(networkInterfaceSpec)
		out.NetworkInterfaces = append(out.NetworkInterfaces, networkInterface)
	}

	out.ReadinessProbe = convert_v1alpha2_ReadinessProbeSpec_To_v1alpha1_Probe(in.ReadinessProbe)
	out.AdvancedOptions = convert_v1alpha2_VirtualMachineAdvancedSpec_To_v1alpha1_VirtualMachineAdvancedOptions(in.Advanced)
	out.ResourcePolicyName = in.Reserved.ResourcePolicyName

	// TODO = in.ReadinessGates

	// Deprecated:
	// out.Ports

	return autoConvert_v1alpha2_VirtualMachineSpec_To_v1alpha1_VirtualMachineSpec(in, out, s)
}

func Convert_v1alpha1_VirtualMachineVolumeStatus_To_v1alpha2_VirtualMachineVolumeStatus(
	in *VirtualMachineVolumeStatus, out *v1alpha2.VirtualMachineVolumeStatus, s apiconversion.Scope) error {

	out.DiskUUID = in.DiskUuid

	return autoConvert_v1alpha1_VirtualMachineVolumeStatus_To_v1alpha2_VirtualMachineVolumeStatus(in, out, s)
}

func Convert_v1alpha2_VirtualMachineVolumeStatus_To_v1alpha1_VirtualMachineVolumeStatus(
	in *v1alpha2.VirtualMachineVolumeStatus, out *VirtualMachineVolumeStatus, s apiconversion.Scope) error {

	out.DiskUuid = in.DiskUUID

	return autoConvert_v1alpha2_VirtualMachineVolumeStatus_To_v1alpha1_VirtualMachineVolumeStatus(in, out, s)
}

func Convert_v1alpha1_VirtualMachineStatus_To_v1alpha2_VirtualMachineStatus(
	in *VirtualMachineStatus, out *v1alpha2.VirtualMachineStatus, s apiconversion.Scope) error {

	out.Network = convert_v1alpha1_Network_To_v1alpha2_NetworkStatus(in.VmIp, in.NetworkInterfaces)

	// WARNING: in.Phase requires manual conversion: does not exist in peer-type

	return autoConvert_v1alpha1_VirtualMachineStatus_To_v1alpha2_VirtualMachineStatus(in, out, s)
}

func Convert_v1alpha2_VirtualMachineStatus_To_v1alpha1_VirtualMachineStatus(
	in *v1alpha2.VirtualMachineStatus, out *VirtualMachineStatus, s apiconversion.Scope) error {

	out.VmIp, out.NetworkInterfaces = convert_v1alpha2_NetworkStatus_To_v1alpha1_Network(in.Network)

	// WARNING: in.Image requires manual conversion: does not exist in peer-type
	// WARNING: in.Class requires manual conversion: does not exist in peer-type

	return autoConvert_v1alpha2_VirtualMachineStatus_To_v1alpha1_VirtualMachineStatus(in, out, s)
}

// ConvertTo converts this VirtualMachine to the Hub version.
func (src *VirtualMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.VirtualMachine)
	if err := Convert_v1alpha1_VirtualMachine_To_v1alpha2_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	// TODO: Manually restore data.
	return nil
}

// ConvertFrom converts the hub version to this VirtualMachine.
func (dst *VirtualMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.VirtualMachine)
	if err := Convert_v1alpha2_VirtualMachine_To_v1alpha1_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	// TODO: Preserve Hub data on down-conversion.
	return nil
}

// ConvertTo converts this VirtualMachineList to the Hub version.
func (src *VirtualMachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.VirtualMachineList)
	return Convert_v1alpha1_VirtualMachineList_To_v1alpha2_VirtualMachineList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineList.
func (dst *VirtualMachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.VirtualMachineList)
	return Convert_v1alpha2_VirtualMachineList_To_v1alpha1_VirtualMachineList(src, dst, nil)
}
