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
package importpolicy

import (
	submv1 "github.com/submariner-io/lighthouse/pkg/apis/submariner.io/v1"
	"k8s.io/klog"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

type Ruleset struct {
	Policies   map[string]*submv1.ImportPolicy
	All        map[string]*submv1.ImportPolicy
	Namespaces map[string]*submv1.ImportPolicy
	Named      map[string]*submv1.ImportPolicy
	None       map[string]*submv1.ImportPolicy
}

func New() *Ruleset {
	return &Ruleset{
		Policies:   make(map[string]*submv1.ImportPolicy),
		All:        make(map[string]*submv1.ImportPolicy),
		Namespaces: make(map[string]*submv1.ImportPolicy),
		Named:      make(map[string]*submv1.ImportPolicy),
		None:       make(map[string]*submv1.ImportPolicy),
	}
}

func (r *Ruleset) Add(policy *submv1.ImportPolicy) error {
	key := keyFunc(policy.Namespace, policy.Name)
	r.Policies[key] = policy
	switch policy.Spec.Policy {
	case submv1.ImportAll:
		r.All[key] = policy
	case submv1.ImportNamespace:
		r.Namespaces[policy.Spec.Namespace] = policy
	case submv1.ImportNamed:
		name := keyFunc(policy.Spec.Namespace, policy.Spec.Name)
		r.Named[name] = policy
	case submv1.ImportNone:
		r.None[key] = policy
	}
	return nil
}

func (r *Ruleset) Update(old, new *submv1.ImportPolicy) error {
	r.Delete(old)
	r.Add(new)
	return nil
}

func (r *Ruleset) Delete(policy *submv1.ImportPolicy) error {
	key := keyFunc(policy.Namespace, policy.Name)
	found := r.Policies[key]
	if found == nil {
		klog.Errorf("Failed to find ImportPolicy %v", key)
		return nil
	}
	delete(r.Policies, key)

	switch policy.Spec.Policy {
	case submv1.ImportAll:
		delete(r.All, key)
	case submv1.ImportNamespace:
		delete(r.Namespaces, policy.Spec.Namespace)
	case submv1.ImportNamed:
		name := keyFunc(policy.Spec.Namespace, policy.Spec.Name)
		delete(r.Named, name)
	case submv1.ImportNone:
		delete(r.None, key)
	}
	return nil
}

func (r *Ruleset) ApplyPolicy(serviceImport *mcsv1a1.ServiceImport) (*mcsv1a1.ServiceImport, bool) {
	if len(r.None) > 0 {
		return nil, false
	}
	if len(r.All) > 0 {
		return serviceImport, true
	}
	if len(r.Namespaces) > 0 {
		policy := r.Namespaces[serviceImport.Namespace]
		if policy != nil {
			if policy.Spec.TargetNamespace != "" {
				serviceImport.Namespace = policy.Spec.TargetNamespace
			}
			return serviceImport, true
		}
	}
	if len(r.Named) > 0 {
		name := keyFunc(serviceImport.Namespace, serviceImport.Name)
		policy := r.Named[name]
		if policy != nil {
			if policy.Spec.TargetNamespace != "" {
				serviceImport.Namespace = policy.Spec.TargetNamespace
			}
			return serviceImport, true
		}
	}
	return nil, false
}

func keyFunc(namespace, name string) string {
	return namespace + "/" + name
}
