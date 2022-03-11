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
package importpolicy_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	submv1 "github.com/submariner-io/lighthouse/pkg/apis/submariner.io/v1"
	"github.com/submariner-io/lighthouse/pkg/importpolicy"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

var _ = Describe("ImportPolicy Ruleset", func() {

	ruleset := importpolicy.New()
	BeforeEach(func() {
		ruleset = importpolicy.New()
	})

	allRule := func(ns, name string) *submv1.ImportPolicy {
		return &submv1.ImportPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name: name,
			},
			Spec: submv1.ImportPolicySpec{
				Policy: submv1.ImportAll,
			},
		}
	}

	noneRule := func(ns, name string) *submv1.ImportPolicy {
		return &submv1.ImportPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name: name,
			},
			Spec: submv1.ImportPolicySpec{
				Policy: submv1.ImportNone,
			},
		}
	}

	namespaceRule := func(ns, name, namespace, targetNamespace string) *submv1.ImportPolicy {
		return &submv1.ImportPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name: name,
			},
			Spec: submv1.ImportPolicySpec{
				Policy: submv1.ImportNamespace,
				Namespace: namespace,
				TargetNamespace: targetNamespace,
			},
		}
	}

	namedRule := func(ruleNs, ruleName, namespace, name, targetNamespace string) *submv1.ImportPolicy {
		return &submv1.ImportPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ruleNs,
				Name: ruleName,
			},
			Spec: submv1.ImportPolicySpec{
				Policy: submv1.ImportNamed,
				Namespace: namespace,
				Name: name,
				TargetNamespace: targetNamespace,
			},
		}
	}

	serviceImport := func(ns, name string) *mcsv1a1.ServiceImport {
		return &mcsv1a1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name: name,
			},
		}
	}

	When("import-all policy rule is configured", func() {
		BeforeEach(func() {
			ruleset.Add(allRule("submariner-operator", "import-all"))
		})

		It("should allow all imports", func() {
			input := serviceImport("test", "iperf3")
			result, success := ruleset.ApplyPolicy(input)
			Expect(success).To(BeTrue())
			Expect(result).To(Equal(input))
		})
	})

	When("import-none policy rule is configured", func() {
		BeforeEach(func() {
			ruleset.Add(noneRule("submariner-operator", "import-none"))
		})

		It("should prevent all imports", func() {
			input := serviceImport("test", "iperf3")
			result, success := ruleset.ApplyPolicy(input)
			Expect(success).To(BeFalse())
			Expect(result).To(BeNil())
		})
	})

	When("import-namespace policy rule is configured", func() {
		BeforeEach(func() {
			ruleset.Add(namespaceRule("submariner-operator", "import-test-namespace", "test", ""))
		})

		It("should allow imports from named namespaces", func() {
			input := serviceImport("test", "iperf3")
			result, success := ruleset.ApplyPolicy(input)
			Expect(success).To(BeTrue())
			Expect(result).To(Equal(input))
		})

		It("should prevent imports from other namespaces", func() {
			input := serviceImport("other", "iperf3")
			result, success := ruleset.ApplyPolicy(input)
			Expect(success).To(BeFalse())
			Expect(result).To(BeNil())
		})
	})

	When("import-namespace policy is configured with a targetNamespace", func() {
		BeforeEach(func() {
			ruleset.Add(namespaceRule("submariner-operator", "import-test-namespace", "test", "custom"))
		})

		It("should allow imports from namespace and perform mapping", func() {
			input := serviceImport("test", "iperf3")
			expected := serviceImport("custom", "iperf3")
			result, success := ruleset.ApplyPolicy(input)
			Expect(success).To(BeTrue())
			Expect(result).To(Equal(expected))
		})

		It("should prevent imports from other namespaces", func() {
			input := serviceImport("other", "iperf3")
			result, success := ruleset.ApplyPolicy(input)
			Expect(success).To(BeFalse())
			Expect(result).To(BeNil())
		})
	})

	When("import-named policy is configured", func() {
		BeforeEach(func() {
			ruleset.Add(namedRule("submariner-operator", "import-test-named", "test", "iperf3", ""))
		})

		It("should allow specific import", func() {
			input := serviceImport("test", "iperf3")
			result, success := ruleset.ApplyPolicy(input)
			Expect(success).To(BeTrue())
			Expect(result).To(Equal(input))
		})

		It("should prevent other imports from same namespace", func() {
			input := serviceImport("test", "other")
			result, success := ruleset.ApplyPolicy(input)
			Expect(success).To(BeFalse())
			Expect(result).To(BeNil())
		})

		It("should prevent imports from other namespaces", func() {
			input := serviceImport("other", "iperf3")
			result, success := ruleset.ApplyPolicy(input)
			Expect(success).To(BeFalse())
			Expect(result).To(BeNil())
		})
	})

	When("import-named policy is configured with a targetNamespace", func() {
		BeforeEach(func() {
			ruleset.Add(namedRule("submariner-operator", "import-test-named", "test", "iperf3", "custom"))
		})

		It("should allow specific import", func() {
			input := serviceImport("test", "iperf3")
			expected := serviceImport("custom", "iperf3")
			result, success := ruleset.ApplyPolicy(input)
			Expect(success).To(BeTrue())
			Expect(result).To(Equal(expected))
		})

		It("should prevent other imports from same namespace", func() {
			input := serviceImport("test", "other")
			result, success := ruleset.ApplyPolicy(input)
			Expect(success).To(BeFalse())
			Expect(result).To(BeNil())
		})

		It("should prevent imports from other namespaces", func() {
			input := serviceImport("other", "iperf3")
			result, success := ruleset.ApplyPolicy(input)
			Expect(success).To(BeFalse())
			Expect(result).To(BeNil())
		})
	})
})
