/*
Copyright 2020 Critical Stack, LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	machinev1 "github.com/criticalstack/machine-api/api/v1alpha1"
	mapierrors "github.com/criticalstack/machine-api/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MachineFinalizer = "awsmachine.infrastructure.crit.sh"

	NodeOwnerLabelName = "infrastructure.crit.sh/awsmachine"
)

// AWSMachineSpec defines the desired state of AWSMachine
type AWSMachineSpec struct {
	// +optional
	ProviderID   *string                 `json:"providerID"`
	AMI          string                  `json:"ami,omitempty"`
	BlockDevices []AWSBlockDeviceMapping `json:"blockDevices,omitempty"`
	InstanceType string                  `json:"instanceType,omitempty"`
	// +optional
	IAMInstanceProfile string `json:"iamInstanceProfile,omitempty"`
	// +optional
	KeyName string `json:"keyName,omitempty"`
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
	// +optional
	SecurityGroupIDs []string `json:"securityGroupIDs,omitempty"`
	// +optional
	SecurityGroupNames []string `json:"securityGroupNames,omitempty"`
	// +optional
	AvailabilityZone string `json:"availabilityZone,omitempty"`
	// +optional
	Region string `json:"region,omitempty"`
	// +optional
	SubnetIDs []string `json:"subnetIDs,omitempty"`
	// +optional
	PublicIP bool `json:"publicIP,omitempty"`
	// +optional
	VPCID string `json:"vpcID,omitempty"`
	// +optional
	SecretRef *corev1.ObjectReference `json:"secretRef,omitempty"`

	// TODO(chrism): needs to be implemented
	// FailureDomain is the failure domain unique identifier this Machine
	// should be attached to, as defined in Cluster API. For this
	// infrastructure provider, the ID is equivalent to an AWS Availability
	// Zone. If multiple subnets are matched for the availability zone, the
	// first one returned is picked.
	FailureDomain *string `json:"failureDomain,omitempty"`
}

type AWSBlockDeviceMapping struct {
	DeviceName string `json:"deviceName,omitempty"`
	VolumeSize int64  `json:"volumeSize,omitempty"`
	VolumeType string `json:"volumeType,omitempty"`
	// +optional
	Encrypted bool `json:"encrypted,omitempty"`
}

// AWSMachineStatus defines the observed state of AWSMachine
type AWSMachineStatus struct {
	// Ready is true when the provider resource is ready.
	// +optional
	Ready bool `json:"ready"`

	// Addresses contains the AWS instance associated addresses.
	Addresses     machinev1.MachineAddresses `json:"addresses,omitempty"`
	InstanceState string                     `json:"instanceState,omitempty"`
	// FailureReason will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a succinct value suitable
	// for machine interpretation.
	//
	// This field should not be set for transitive errors that a controller
	// faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	// +optional
	FailureReason *mapierrors.MachineStatusError `json:"failureReason,omitempty"`

	// FailureMessage will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a more verbose string suitable
	// for logging and human consumption.
	//
	// This field should not be set for transitive errors that a controller
	// faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`
}

func (m *AWSMachineStatus) SetFailure(err mapierrors.MachineStatusError, msg string) {
	m.FailureReason = &err
	m.FailureMessage = &msg
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=awsmachines,scope=Namespaced,categories=machine-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.instanceState",description="EC2 instance state"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Machine ready status"
// +kubebuilder:printcolumn:name="InstanceID",type="string",JSONPath=".spec.providerID",description="EC2 instance ID"
// +kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.ownerReferences[?(@.kind==\"Machine\")].name",description="Machine object which owns with this AWSMachine"

// AWSMachine is the Schema for the awsmachines API
type AWSMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AWSMachineSpec   `json:"spec,omitempty"`
	Status AWSMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AWSMachineList contains a list of AWSMachine
type AWSMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AWSMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AWSMachine{}, &AWSMachineList{})
}
