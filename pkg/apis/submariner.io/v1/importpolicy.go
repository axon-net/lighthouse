/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the Submariner project.

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

// ImportPolicySpec defines the desired state of ImportPolicy
type ImportPolicySpec struct {

	// ClusterSelector identifies the Clusters to which the import policy should be applied.
	// An empty selector indicates wildcard, ie matches any cluster.
	ClusterSelector metav1.LabelSelector `json:"clusterSelector,omitempty"`

	// The desired import behaviour.
	Policy          PolicyType `json:"policy"`

	// Name of the service to import. Only valid for policy=Named
	// +optional
	Name            string     `json:"name,omitempty"`

	// Namspace to import services from. Valid for policy=Namespace|Named
	// +optional
	Namespace       string     `json:"namespace,omitempty"`

	// Target namespace to import services into. Valid for policy=Namespace|Named
	// +optional
	TargetNamespace string     `json:"targetNamespace,omitempty"`
}

// +kubebuilder:validation:Enum=ImportAll;Namespace;Named;ImportNone
type PolicyType string

const (
	ImportAll       PolicyType = "ImportAll"
	ImportNamespace PolicyType = "Namespace"
	ImportNamed     PolicyType = "Named"
	ImportNone      PolicyType = "ImportNone"
)

// +genclient
// +genclient:noStatus
// +kubebuilder:object:root=true

type ImportPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ImportPolicySpec `json:"spec"`
}

// +kubebuilder:object:root=true

type ImportPolicyList struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items             []ImportPolicy `json:"items"`
}

func init() {
        SchemeBuilder.Register(&ImportPolicy{}, &ImportPolicyList{})
}
