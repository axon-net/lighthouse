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
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	submv1 "github.com/submariner-io/lighthouse/pkg/apis/submariner.io/v1"
	submClientv1 "github.com/submariner-io/lighthouse/pkg/client/clientset/versioned"
)

type Controller struct {
	// Indirection hook for unit tests to supply fake client sets
	NewClientset   func() (*submClientv1.Clientset, error)
	svcInformer    cache.Controller
	svcStore       cache.Store
	localClusterID string
	policyRules *Ruleset
}

func NewController(localClusterID string, ruleset *Ruleset, restConfig *rest.Config) *Controller {
	return &Controller{
		NewClientset: func() (*submClientv1.Clientset, error) {
			return submClientv1.NewForConfig(restConfig) // nolint:wrapcheck // Let the caller wrap it.
		},
		localClusterID: localClusterID,
		policyRules: ruleset,
	}
}

func (c *Controller) Start(stopCh <-chan struct{}) error {
	klog.Infof("Starting ImportPolicy Controller")

	clientSet, err := c.NewClientset()
	if err != nil {
		return errors.Wrap(err, "error creating client set")
	}

	// nolint:wrapcheck // Let the caller wrap these errors.
	c.svcStore, c.svcInformer = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return clientSet.SubmarinerV1().ImportPolicies(metav1.NamespaceAll).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return clientSet.SubmarinerV1().ImportPolicies(metav1.NamespaceAll).Watch(context.TODO(), options)
			},
		},
		&submv1.ImportPolicy{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { c.policyRules.Add(obj.(*submv1.ImportPolicy)) },
			UpdateFunc: func(old interface{}, new interface{}) {
				c.policyRules.Update(old.(*submv1.ImportPolicy), new.(*submv1.ImportPolicy))
			},
			DeleteFunc: func(obj interface{}) { c.policyRules.Delete(obj.(*submv1.ImportPolicy)) },
		},
	)

	go c.svcInformer.Run(stopCh)

	return nil
}
