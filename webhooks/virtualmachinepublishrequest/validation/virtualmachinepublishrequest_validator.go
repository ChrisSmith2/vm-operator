// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"

	"github.com/pkg/errors"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"

	APIVersionNotSupported = "API version %s isn't supported"
	KindNotSupported       = "kind %s isn't supported"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha1-virtualmachinepublishrequest,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinepublishrequests,versions=v1alpha1,name=default.validating.virtualmachinepublishrequest.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinepublishrequests,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinepublishrequests/status,verbs=get
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraries,verbs=get;list;

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return errors.Wrapf(err, "failed to create VirtualMachinePublishRequest validation webhook")
	}
	mgr.GetWebhookServer().Register(hook.Path, hook)

	return nil
}

// NewValidator returns the package's Validator.
func NewValidator(client client.Client) builder.Validator {
	return validator{
		client:    client,
		converter: runtime.DefaultUnstructuredConverter,
	}
}

type validator struct {
	client    client.Client
	converter runtime.UnstructuredConverter
}

func (v validator) For() schema.GroupVersionKind {
	return vmopv1.SchemeGroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachinePublishRequest{}).Name())
}

func (v validator) ValidateCreate(ctx *context.WebhookRequestContext) admission.Response {
	if !lib.IsWCPVMImageRegistryEnabled() {
		return common.BuildValidationResponse(ctx, []string{"WCP_VM_Image_Registry feature not enabled"}, nil)
	}

	vmpub, err := v.vmPublishRequestFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, v.validateSource(ctx, vmpub)...)
	fieldErrs = append(fieldErrs, v.validateTargetLocation(ctx, vmpub)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) ValidateDelete(*context.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

func (v validator) ValidateUpdate(ctx *context.WebhookRequestContext) admission.Response {
	vmpub, err := v.vmPublishRequestFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldVMpub, err := v.vmPublishRequestFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	// Check if an immutable field has been modified.
	fieldErrs = append(fieldErrs, v.validateImmutableFields(vmpub, oldVMpub)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) validateSource(ctx *context.WebhookRequestContext, vmpub *vmopv1.VirtualMachinePublishRequest) field.ErrorList {
	var allErrs field.ErrorList

	sourcePath := field.NewPath("spec").Child("source")
	if apiVersion := vmpub.Spec.Source.APIVersion; apiVersion != vmopv1.SchemeGroupVersion.String() && apiVersion != "" {
		allErrs = append(allErrs, field.NotSupported(sourcePath.Child("apiVersion"),
			vmpub.Spec.Source.APIVersion, []string{vmopv1.SchemeGroupVersion.String(), ""}))
	}

	if kind := vmpub.Spec.Source.Kind; kind != reflect.TypeOf(vmopv1.VirtualMachine{}).Name() && kind != "" {
		allErrs = append(allErrs, field.NotSupported(sourcePath.Child("kind"),
			vmpub.Spec.Source.Kind, []string{reflect.TypeOf(vmopv1.VirtualMachine{}).Name(), ""}))
	}

	if len(allErrs) != 0 {
		return allErrs
	}

	vmName := vmpub.Spec.Source.Name
	defaultSourceVM := false
	if vmName == "" {
		vmName = vmpub.Name
		defaultSourceVM = true
	}

	vm := &vmopv1.VirtualMachine{}
	if err := v.client.Get(ctx.Context, client.ObjectKey{Name: vmName, Namespace: vmpub.Namespace}, vm); err != nil {
		if apiErrors.IsNotFound(err) && !defaultSourceVM {
			return append(allErrs, field.NotFound(sourcePath.Child("name"), vmName))
		}

		// Build error messages
		errMsg := err.Error()
		if vmpub.Spec.Source.Name == "" {
			errMsg = fmt.Sprintf("failed to get the default source VM with vmpub request name: %s, %s", vmName, errMsg)
		}
		return append(allErrs, field.Invalid(sourcePath.Child("name"), vmpub.Spec.Source.Name, errMsg))
	}

	return allErrs
}

func (v validator) validateTargetLocation(ctx *context.WebhookRequestContext, vmpub *vmopv1.VirtualMachinePublishRequest) field.ErrorList {
	var allErrs field.ErrorList

	targetLocationPath := field.NewPath("spec").Child("target").
		Child("location")
	targetLocationName := vmpub.Spec.Target.Location.Name
	targetLocationNamePath := targetLocationPath.Child("name")
	if targetLocationName == "" {
		allErrs = append(allErrs, field.Required(targetLocationNamePath, ""))
	}

	if vmpub.Spec.Target.Location.APIVersion != imgregv1a1.GroupVersion.String() {
		allErrs = append(allErrs, field.NotSupported(targetLocationPath.Child("apiVersion"),
			vmpub.Spec.Target.Location.APIVersion, []string{imgregv1a1.GroupVersion.String(), ""}))
	}

	if vmpub.Spec.Target.Location.Kind != reflect.TypeOf(imgregv1a1.ContentLibrary{}).Name() {
		allErrs = append(allErrs, field.NotSupported(targetLocationPath.Child("kind"),
			vmpub.Spec.Target.Location.Kind, []string{reflect.TypeOf(imgregv1a1.ContentLibrary{}).Name(), ""}))
	}

	if len(allErrs) != 0 {
		return allErrs
	}

	// Validate the target location content library should be writable.
	cl := &imgregv1a1.ContentLibrary{}
	if err := v.client.Get(ctx.Context, client.ObjectKey{Name: targetLocationName,
		Namespace: vmpub.Namespace}, cl); err != nil {
		if apiErrors.IsNotFound(err) {
			return append(allErrs, field.NotFound(targetLocationNamePath, targetLocationName))
		}
		return append(allErrs, field.Invalid(targetLocationNamePath, targetLocationName, err.Error()))
	}

	if !cl.Spec.Writable {
		allErrs = append(allErrs, field.Invalid(targetLocationNamePath, targetLocationName,
			fmt.Sprintf("target location %s is not writable", targetLocationName)))
	}

	return allErrs
}

func (v validator) validateImmutableFields(vmpub, oldvmpub *vmopv1.VirtualMachinePublishRequest) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// all updates to source and target are not allowed.
	// Otherwise, we may end up in a situation where multiple OVFs are published for a single VMPub.
	allErrs = append(allErrs, validation.ValidateImmutableField(vmpub.Spec.Source, oldvmpub.Spec.Source, specPath.Child("source"))...)
	allErrs = append(allErrs, validation.ValidateImmutableField(vmpub.Spec.Target, oldvmpub.Spec.Target, specPath.Child("target"))...)

	return allErrs
}

// vmPublishRequestFromUnstructured returns the VirtualMachineService from the unstructured object.
func (v validator) vmPublishRequestFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachinePublishRequest, error) {
	vmPubReq := &vmopv1.VirtualMachinePublishRequest{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), vmPubReq); err != nil {
		return nil, err
	}
	return vmPubReq, nil
}
