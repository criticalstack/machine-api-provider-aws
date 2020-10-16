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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AWSInfrastructureProviderSpec defines the desired state of AWSInfrastructureProvider
type AWSInfrastructureProviderSpec struct {
	Region string `json:"region"`
	// other stuff
}

// InfrastructureProviderStatus defines the observed state of AWSInfrastructureProvider
type AWSInfrastructureProviderStatus struct {
	Ready       bool        `json:"ready"`
	LastUpdated metav1.Time `json:"lastUpdated"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=awsinfrastructureproviders,scope=Namespaced,categories=machine-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Provider is ready"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AWSInfrastructureProvider is the Schema for the awsinfrastructureproviders API
type AWSInfrastructureProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AWSInfrastructureProviderSpec   `json:"spec,omitempty"`
	Status AWSInfrastructureProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InfrastructureProviderList contains a list of AWSInfrastructureProvider
type AWSInfrastructureProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AWSInfrastructureProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AWSInfrastructureProvider{}, &AWSInfrastructureProviderList{})
}
