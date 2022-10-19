// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	goctx "context"
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmUtilTests() {

	var (
		k8sClient   client.Client
		initObjects []client.Object

		vmCtx context.VirtualMachineContext
	)

	BeforeEach(func() {
		vm := builder.DummyBasicVirtualMachine("test-vm", "dummy-ns")

		vmCtx = context.VirtualMachineContext{
			Context: goctx.Background(),
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}
	})

	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClient(initObjects...)
	})

	AfterEach(func() {
		k8sClient = nil
		initObjects = nil
	})

	Context("GetVirtualMachineClass", func() {

		var (
			vmClass        *vmopv1alpha1.VirtualMachineClass
			vmClassBinding *vmopv1alpha1.VirtualMachineClassBinding
		)

		BeforeEach(func() {
			vmClass, vmClassBinding = builder.DummyVirtualMachineClassAndBinding("dummy-vm-class", vmCtx.VM.Namespace)
			vmCtx.VM.Spec.ClassName = vmClass.Name
		})

		Context("VirtualMachineClass custom resource doesn't exist", func() {
			It("Returns error and sets condition when VM Class does not exist", func() {
				expectedErrMsg := fmt.Sprintf("Failed to get VirtualMachineClass: %s", vmCtx.VM.Spec.ClassName)

				_, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

				expectedCondition := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(
						vmopv1alpha1.VirtualMachinePrereqReadyCondition,
						vmopv1alpha1.VirtualMachineClassNotFoundReason,
						vmopv1alpha1.ConditionSeverityError,
						expectedErrMsg),
				}
				Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
			})
		})

		validateNoVMClassBindingCondition := func(vm *vmopv1alpha1.VirtualMachine) {
			msg := fmt.Sprintf("Namespace %s does not have access to VirtualMachineClass %s", vm.Namespace, vm.Spec.ClassName)

			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.FalseCondition(
					vmopv1alpha1.VirtualMachinePrereqReadyCondition,
					vmopv1alpha1.VirtualMachineClassBindingNotFoundReason,
					vmopv1alpha1.ConditionSeverityError,
					msg),
			}
			Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
		}

		Context("VirtualMachineClass custom resource exists", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, vmClass)
			})

			Context("No VirtualMachineClassBinding exists in namespace", func() {
				It("return an error and sets VirtualMachinePreReqReady Condition to false", func() {
					expectedErr := fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace %s", vmCtx.VM.Spec.ClassName, vmCtx.VM.Namespace)

					_, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(expectedErr))
					validateNoVMClassBindingCondition(vmCtx.VM)
				})
			})

			Context("VirtualMachineBinding is not present for VM Class", func() {
				BeforeEach(func() {
					vmClassBinding.ClassRef.Name = "blah-blah-binding"
					initObjects = append(initObjects, vmClassBinding)
				})

				It("returns an error and sets the VirtualMachinePrereqReady Condition to false", func() {
					expectedErr := fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace %s", vmCtx.VM.Spec.ClassName, vmCtx.VM.Namespace)

					_, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(expectedErr))
					validateNoVMClassBindingCondition(vmCtx.VM)
				})
			})

			Context("VirtualMachineBinding is present for VM Class", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmClassBinding)
				})

				It("returns success", func() {
					class, err := vsphere.GetVirtualMachineClass(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(class).ToNot(BeNil())
				})
			})
		})

	})

	Context("GetVMImageAndContentLibraryUUID", func() {

		var (
			contentSource        *vmopv1alpha1.ContentSource
			clProvider           *vmopv1alpha1.ContentLibraryProvider
			contentSourceBinding *vmopv1alpha1.ContentSourceBinding
			vmImage              *vmopv1alpha1.VirtualMachineImage
		)

		BeforeEach(func() {
			contentSource, clProvider, contentSourceBinding = builder.DummyContentSourceProviderAndBinding("dummy-cl-uuid", vmCtx.VM.Namespace)
			vmImage = &vmopv1alpha1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy-image",
					OwnerReferences: []metav1.OwnerReference{{
						Name: clProvider.Name,
						Kind: "ContentLibraryProvider",
					}},
				},
			}

			vmCtx.VM.Spec.ImageName = vmImage.Name
		})

		When("VirtualMachineImage does not exist", func() {
			It("returns error and sets condition", func() {
				expectedErrMsg := fmt.Sprintf("Failed to get VirtualMachineImage: %s", vmCtx.VM.Spec.ImageName)

				_, _, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

				expectedCondition := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(
						vmopv1alpha1.VirtualMachinePrereqReadyCondition,
						vmopv1alpha1.VirtualMachineImageNotFoundReason,
						vmopv1alpha1.ConditionSeverityError,
						expectedErrMsg),
				}
				Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
			})
		})

		When("ContentLibraryProvider does not exist", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, vmImage)
			})

			It("returns error and sets condition", func() {
				expectedErrMsg := fmt.Sprintf("Failed to get ContentLibraryProvider: %s", clProvider.Name)

				_, _, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

				expectedCondition := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(
						vmopv1alpha1.VirtualMachinePrereqReadyCondition,
						vmopv1alpha1.ContentLibraryProviderNotFoundReason,
						vmopv1alpha1.ConditionSeverityError,
						expectedErrMsg),
				}
				Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
			})
		})

		validateNoContentSourceBindingCondition := func(vm *vmopv1alpha1.VirtualMachine, clUUID string) {
			msg := fmt.Sprintf("Namespace %s does not have access to ContentSource %s for VirtualMachineImage %s",
				vm.Namespace, clUUID, vm.Spec.ImageName)

			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.FalseCondition(
					vmopv1alpha1.VirtualMachinePrereqReadyCondition,
					vmopv1alpha1.ContentSourceBindingNotFoundReason,
					vmopv1alpha1.ConditionSeverityError,
					msg),
			}

			Expect(vmCtx.VM.Status.Conditions).To(conditions.MatchConditions(expectedCondition))
		}

		Context("VirtualMachineImage and ContentLibraryProvider exist", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, clProvider, vmImage)
			})

			When("No ContentSourceBindings exist in the namespace", func() {
				It("return an error and sets VirtualMachinePreReqReady Condition to false", func() {
					expectedErrMsg := fmt.Sprintf("Namespace %s does not have access to ContentSource %s for VirtualMachineImage %s",
						vmCtx.VM.Namespace, clProvider.Spec.UUID, vmCtx.VM.Spec.ImageName)

					_, _, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

					validateNoContentSourceBindingCondition(vmCtx.VM, clProvider.Spec.UUID)
				})
			})

			When("ContentSourceBinding is not present for the content library corresponding to the VM image", func() {
				BeforeEach(func() {
					contentSourceBinding.ContentSourceRef.Name = "blah-blah-binding"
					initObjects = append(initObjects, contentSourceBinding)
				})

				It("return an error and sets VirtualMachinePreReqReady Condition to false", func() {
					expectedErrMsg := fmt.Sprintf("Namespace %s does not have access to ContentSource %s for VirtualMachineImage %s",
						vmCtx.VM.Namespace, clProvider.Spec.UUID, vmCtx.VM.Spec.ImageName)

					_, _, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedErrMsg))

					validateNoContentSourceBindingCondition(vmCtx.VM, clProvider.Spec.UUID)
				})
			})

			When("ContentSourceBinding present for ContentSource", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, contentSource, contentSourceBinding)
				})

				It("returns success", func() {
					image, uuid, err := vsphere.GetVMImageAndContentLibraryUUID(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(image).ToNot(BeNil())
					Expect(uuid).ToNot(BeEmpty())
					Expect(uuid).To(Equal(clProvider.Spec.UUID))
				})
			})
		})
	})

	Context("GetVMMetadata", func() {

		var (
			vmMetaDataConfigMap *corev1.ConfigMap
			vmMetaDataSecret    *corev1.Secret
		)

		BeforeEach(func() {
			vmMetaDataConfigMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-vm-metadata",
					Namespace: vmCtx.VM.Namespace,
				},
				Data: map[string]string{
					"foo": "bar",
				},
			}

			vmMetaDataSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-vm-metadata",
					Namespace: vmCtx.VM.Namespace,
				},
				Data: map[string][]byte{
					"foo": []byte("bar"),
				},
			}
		})

		When("both ConfigMap and Secret are specified", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.VmMetadata = &vmopv1alpha1.VirtualMachineMetadata{
					ConfigMapName: vmMetaDataConfigMap.Name,
					SecretName:    vmMetaDataSecret.Name,
					Transport:     "transport",
				}
			})

			It("returns an error", func() {
				_, err := vsphere.GetVMMetadata(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid VM Metadata"))
			})
		})

		When("VM Metadata is specified via a ConfigMap", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.VmMetadata = &vmopv1alpha1.VirtualMachineMetadata{
					ConfigMapName: vmMetaDataConfigMap.Name,
					Transport:     "transport",
				}
			})

			It("return an error when ConfigMap does not exist", func() {
				md, err := vsphere.GetVMMetadata(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(md).To(Equal(session.VMMetadata{}))
			})

			When("ConfigMap exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmMetaDataConfigMap)
				})

				It("returns success", func() {
					md, err := vsphere.GetVMMetadata(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(md.Data).To(Equal(vmMetaDataConfigMap.Data))
				})
			})
		})

		When("VM Metadata is specified via a Secret", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.VmMetadata = &vmopv1alpha1.VirtualMachineMetadata{
					SecretName: vmMetaDataSecret.Name,
					Transport:  "transport",
				}
			})

			It("returns an error when Secret does not exist", func() {
				md, err := vsphere.GetVMMetadata(vmCtx, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(md).To(Equal(session.VMMetadata{}))
			})

			When("Secret exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmMetaDataSecret)
				})

				It("returns success", func() {
					md, err := vsphere.GetVMMetadata(vmCtx, k8sClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(md.Data).ToNot(BeEmpty())
				})
			})
		})
	})

	Context("GetVMSetResourcePolicy", func() {

		var (
			vmResourcePolicy *vmopv1alpha1.VirtualMachineSetResourcePolicy
		)

		BeforeEach(func() {
			vmResourcePolicy = &vmopv1alpha1.VirtualMachineSetResourcePolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-vm-rp",
					Namespace: vmCtx.VM.Namespace,
				},
				Spec: vmopv1alpha1.VirtualMachineSetResourcePolicySpec{
					ResourcePool: vmopv1alpha1.ResourcePoolSpec{Name: "fooRP"},
					Folder:       vmopv1alpha1.FolderSpec{Name: "fooFolder"},
				},
			}
		})

		It("returns success when VM does not have SetResourcePolicy", func() {
			vmCtx.VM.Spec.ResourcePolicyName = ""
			rp, err := vsphere.GetVMSetResourcePolicy(vmCtx, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Expect(rp).To(BeNil())
		})

		It("VM SetResourcePolicy does not exist", func() {
			vmCtx.VM.Spec.ResourcePolicyName = "bogus"
			rp, err := vsphere.GetVMSetResourcePolicy(vmCtx, k8sClient)
			Expect(err).To(HaveOccurred())
			Expect(rp).To(BeNil())
		})

		When("VM SetResourcePolicy exists", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, vmResourcePolicy)
				vmCtx.VM.Spec.ResourcePolicyName = vmResourcePolicy.Name
			})

			It("returns success", func() {
				rp, err := vsphere.GetVMSetResourcePolicy(vmCtx, k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(rp).ToNot(BeNil())
			})
		})
	})

	Context("AddInstanceStorageVolumes", func() {

		var (
			vmClass            *vmopv1alpha1.VirtualMachineClass
			instanceStorageFSS uint32
		)

		expectInstanceStorageVolumes := func(
			vm *vmopv1alpha1.VirtualMachine,
			isStorage vmopv1alpha1.InstanceStorage) {

			ExpectWithOffset(1, isStorage.Volumes).ToNot(BeEmpty())
			isVolumes := instancestorage.FilterVolumes(vm)
			ExpectWithOffset(1, isVolumes).To(HaveLen(len(isStorage.Volumes)))

			for _, isVol := range isStorage.Volumes {
				found := false

				for idx, vol := range isVolumes {
					claim := vol.PersistentVolumeClaim.InstanceVolumeClaim
					if claim.StorageClass == isStorage.StorageClass && claim.Size == isVol.Size {
						isVolumes = append(isVolumes[:idx], isVolumes[idx+1:]...)
						found = true
						break
					}
				}

				ExpectWithOffset(1, found).To(BeTrue(), "failed to find instance storage volume for %v", isVol)
			}
		}

		BeforeEach(func() {
			lib.IsInstanceStorageFSSEnabled = func() bool {
				return atomic.LoadUint32(&instanceStorageFSS) != 0
			}

			vmClass = builder.DummyVirtualMachineClass()
		})

		AfterEach(func() {
			atomic.StoreUint32(&instanceStorageFSS, 0)
		})

		It("Instance Storage FSS is disabled", func() {
			atomic.StoreUint32(&instanceStorageFSS, 0)

			err := vsphere.AddInstanceStorageVolumes(vmCtx, vmClass)
			Expect(err).ToNot(HaveOccurred())
			Expect(instancestorage.FilterVolumes(vmCtx.VM)).To(BeEmpty())
		})

		When("InstanceStorage FFS is enabled", func() {
			BeforeEach(func() {
				atomic.StoreUint32(&instanceStorageFSS, 1)
			})

			It("VM Class does not contain instance storage volumes", func() {
				err := vsphere.AddInstanceStorageVolumes(vmCtx, vmClass)
				Expect(err).ToNot(HaveOccurred())
				Expect(instancestorage.FilterVolumes(vmCtx.VM)).To(BeEmpty())
			})

			When("Instance Volume is added in VM Class", func() {
				BeforeEach(func() {
					vmClass.Spec.Hardware.InstanceStorage = builder.DummyInstanceStorage()
				})

				It("Instance Volumes should be added", func() {
					err := vsphere.AddInstanceStorageVolumes(vmCtx, vmClass)
					Expect(err).ToNot(HaveOccurred())
					expectInstanceStorageVolumes(vmCtx.VM, vmClass.Spec.Hardware.InstanceStorage)
				})

				It("Instance Storage is already added to VM Spec.Volumes", func() {
					err := vsphere.AddInstanceStorageVolumes(vmCtx, vmClass)
					Expect(err).ToNot(HaveOccurred())

					isVolumesBefore := instancestorage.FilterVolumes(vmCtx.VM)
					expectInstanceStorageVolumes(vmCtx.VM, vmClass.Spec.Hardware.InstanceStorage)

					// Instance Storage is already configured, should not patch again
					err = vsphere.AddInstanceStorageVolumes(vmCtx, vmClass)
					Expect(err).ToNot(HaveOccurred())
					isVolumesAfter := instancestorage.FilterVolumes(vmCtx.VM)
					Expect(isVolumesAfter).To(Equal(isVolumesBefore))
				})
			})
		})
	})
}
