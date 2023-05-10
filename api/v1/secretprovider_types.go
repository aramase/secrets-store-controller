/*
Copyright 2023.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretProviderSpec defines the desired state of SecretProvider
type SecretProviderSpec struct {
	// ServiceAccountName is the name of the service account that will be used to
	// access the secret store.
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// SecretProviderStatus defines the observed state of SecretProvider
type SecretProviderStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// SecretProvider is the Schema for the secretproviders API
type SecretProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecretProviderSpec   `json:"spec,omitempty"`
	Status SecretProviderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SecretProviderList contains a list of SecretProvider
type SecretProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecretProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecretProvider{}, &SecretProviderList{})
}
