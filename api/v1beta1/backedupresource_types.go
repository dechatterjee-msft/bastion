package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackedupResourceSpec defines the desired state of BackedupResource
type BackedupResourceSpec struct {
	// ExampleField is an example field of BackedupResource
	ExampleField string `json:"exampleField,omitempty"`
}

// BackedupResourceStatus defines the observed state of BackedupResource
type BackedupResourceStatus struct {
	// Add status fields here
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// BackedupResource is the Schema for the backedupresources API
type BackedupResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackedupResourceSpec   `json:"spec,omitempty"`
	Status BackedupResourceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BackedupResourceList contains a list of BackedupResource
type BackedupResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackedupResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackedupResource{}, &BackedupResourceList{})
}
