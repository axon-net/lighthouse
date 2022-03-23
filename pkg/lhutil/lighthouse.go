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

package lhutil

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	lhconst "github.com/submariner-io/lighthouse/pkg/constants"
)

// MaxExportStatusConditions Maximum number of conditions to keep in ServiceExport.Status at the spoke (FIFO)
var MaxExportStatusConditions = 10

// GenerateObjectName returns a canonical representation for a fully qualified object.
// The name should be treated as opaque (i.e., no assumption on the order or
// composition of the parts should be made by the caller).
// @todo consider handling when generated object name exceeds k8s limits
//	(e.g., use a crypto hash instead).
func GenerateObjectName(name, ns, cluster string) string {
	return name + "-" + ns + "-" + cluster
}

// GetOriginalObjectName retrieves the original object name based on the
// Lighthouse labels provided in the object metadata.
func GetOriginalObjectName(objmd metav1.ObjectMeta) types.NamespacedName {
	labels := objmd.GetLabels()

	return types.NamespacedName{
		Namespace: labels[lhconst.LabelSourceNamespace],
		Name:      labels[lhconst.LighthouseLabelSourceName],
	}
}

// GetOriginalObjectCluster retrieves the original object cluster
// identifier based on the corresponding Lighthouse label.
func GetOriginalObjectCluster(objmd metav1.ObjectMeta) string {
	return objmd.GetLabels()[lhconst.LighthouseLabelSourceCluster]
}

// Label the object with the expected source object identity labels.
// Note: metav1.SetMetaDataLabel added in v0.20.0
func Label(objmd *metav1.ObjectMeta, name, ns, cluster string) {
	labels := objmd.GetLabels()

	labels[lhconst.LighthouseLabelSourceCluster] = cluster
	labels[lhconst.LabelSourceNamespace] = ns
	labels[lhconst.LighthouseLabelSourceName] = name
}

// Annotate the object with the expected source object identity annotations.
func Annotate(objmd *metav1.ObjectMeta, name, ns, cluster string) {
	metav1.SetMetaDataAnnotation(objmd, lhconst.OriginNamespace, ns)
	metav1.SetMetaDataAnnotation(objmd, lhconst.OriginName, name)
}

// ListFilter creates a filter that can be used to list only matching ServiceExport
// or ServiceImport objects. It relies on the presence of the Lighthouse labels.
func NewServiceExportListFilter(objmd metav1.ObjectMeta) (*client.ListOptions, error) {
	labels := objmd.GetLabels()

	if labels[lhconst.LabelSourceNamespace] == "" || labels[lhconst.LighthouseLabelSourceName] == "" {
		return nil, fmt.Errorf("%s missing lighthouse labels", objmd.GetName())
	}

	opts := &client.ListOptions{}

	client.InNamespace(objmd.GetNamespace()).ApplyToList(opts)
	client.MatchingLabels{
		lhconst.LabelSourceNamespace:      labels[lhconst.LabelSourceNamespace],
		lhconst.LighthouseLabelSourceName: labels[lhconst.LighthouseLabelSourceName],
	}.ApplyToList(opts)
	return opts, nil
}

func GetServiceExportCondition(status *mcsv1a1.ServiceExportStatus, ct mcsv1a1.ServiceExportConditionType) *mcsv1a1.ServiceExportCondition {
	var latestCond *mcsv1a1.ServiceExportCondition = nil
	for _, c := range status.Conditions {
		if c.Type == ct && (latestCond == nil || !c.LastTransitionTime.Before(latestCond.LastTransitionTime)) {
			latestCond = &c
		}
	}

	return latestCond
}

// check if two serviceExportConditions are equal
func ServiceExportConditionEqual(c1, c2 *mcsv1a1.ServiceExportCondition) bool {
	return c1.Type == c2.Type && c1.Status == c2.Status && c1.Reason == c2.Reason &&
		c1.Message == c2.Message
}

func CreateServiceExportCondition(ct mcsv1a1.ServiceExportConditionType, cs corev1.ConditionStatus, reason, msg string) *mcsv1a1.ServiceExportCondition {
	now := metav1.Now()
	return &mcsv1a1.ServiceExportCondition{
		Type:               ct,
		Status:             cs,
		LastTransitionTime: &now,
		Reason:             &reason,
		Message:            &msg,
	}
}

// update the condition field under serviceExport status
func UpdateServiceExportConditions(ctx context.Context, se *mcsv1a1.ServiceExport, log logr.Logger,
	conditionType mcsv1a1.ServiceExportConditionType, status corev1.ConditionStatus, reason string, msg string) error {
	now := metav1.Now()
	exportCondition := mcsv1a1.ServiceExportCondition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: &now,
		Reason:             (*string)(&reason),
		Message:            &msg,
	}

	numCond := len(se.Status.Conditions)
	if numCond > 0 && ServiceExportConditionEqual(&se.Status.Conditions[numCond-1], &exportCondition) {
		lastCond := se.Status.Conditions[numCond-1]
		log.Info("Last ServiceExportCondition equal - not updating status",
			"condition", lastCond)
		return nil
	}

	if numCond >= MaxExportStatusConditions {
		for i, cond := range se.Status.Conditions {
			if cond.Type == conditionType {
				copy(se.Status.Conditions[i:], se.Status.Conditions[i+1:])
				se.Status.Conditions = se.Status.Conditions[:MaxExportStatusConditions]
				se.Status.Conditions[MaxExportStatusConditions-1] = exportCondition
				return nil
			}
		}
		copy(se.Status.Conditions[0:], se.Status.Conditions[1:])
		se.Status.Conditions = se.Status.Conditions[:MaxExportStatusConditions]
		se.Status.Conditions[MaxExportStatusConditions-1] = exportCondition
	} else {
		se.Status.Conditions = append(se.Status.Conditions, exportCondition)
	}

	return nil
}
