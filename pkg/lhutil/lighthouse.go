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
	"fmt"
	"reflect"

	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	lhconst "github.com/submariner-io/lighthouse/pkg/constants"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type ServiceExportConditionReason string

const (
	ReasonEmpty                  ServiceExportConditionReason = "NA"
	ReasonServiceUnavailable     ServiceExportConditionReason = "ServiceUnavailable"
	ReasonAwaitingSync           ServiceExportConditionReason = "AwaitingSync"
	ReasonUnsupportedServiceType ServiceExportConditionReason = "UnsupportedServiceType"
)

var (
	// MaxExportStatusConditions Maximum number of conditions to keep in ServiceExport.Status at the spoke (FIFO)
	MaxExportStatusConditions = 10
	logger                    = logf.Log.WithName("agent")
)

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
func ServiceExportListFilter(objmd metav1.ObjectMeta) (*client.ListOptions, error) {
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

/*
func UpdateExportedServiceStatus(name, namespace string, client client.Client, scheme *runtime.Scheme,
	conditionType mcsv1a1.ServiceExportConditionType, status corev1.ConditionStatus,
	reason ServiceExportConditionReason, msg string) {

	seLog := logger.WithValues("name", namespace+"/"+name)

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		seLog.V(log.DEBUG).Info("Updating local service export status",
			"type", mcsv1a1.ServiceExportValid,
			"status", status,
			"reason", reason,
			"message", msg)

		toUpdate, err := getServiceExport(name, namespace, client, scheme)
		if apierrors.IsNotFound(err) {
			seLog.Info("ServiceExport not found - unable to update status")
			return nil
		} else if err != nil {
			return err
		}

		now := metav1.Now()
		exportCondition := mcsv1a1.ServiceExportCondition{
			Type:               conditionType,
			Status:             status,
			LastTransitionTime: &now,
			Reason:             (*string)(&reason),
			Message:            &msg,
		}

		numCond := len(toUpdate.Status.Conditions)
		if numCond > 0 && serviceExportConditionEqual(&toUpdate.Status.Conditions[numCond-1], &exportCondition) {
			lastCond := toUpdate.Status.Conditions[numCond-1]
			seLog.V(log.TRACE).Info("Last ServiceExportCondition equal - not updating status",
				"condition", lastCond)
			return nil
		}

		if numCond >= MaxExportStatusConditions {
			copy(toUpdate.Status.Conditions[0:], toUpdate.Status.Conditions[1:])
			toUpdate.Status.Conditions = toUpdate.Status.Conditions[:MaxExportStatusConditions]
			toUpdate.Status.Conditions[MaxExportStatusConditions-1] = exportCondition
		} else {
			toUpdate.Status.Conditions = append(toUpdate.Status.Conditions, exportCondition)
		}

		raw, err := resource.ToUnstructured(toUpdate)
		if err != nil {
			err := errors.Wrap(err, "error converting resource")
			return err
		}
		//a.serviceExportClient.Namespace
		_, err = client.Namespace(toUpdate.Namespace).UpdateStatus(context.TODO(), raw, metav1.UpdateOptions{})

		if err != nil {
			err := errors.Wrap(err, "Failed updating service export")
			seLog.V(log.DEBUG).Info(err.Error()) // ensure this is logged in case of a conflict
			return err
		}

		return nil
	})

	if retryErr != nil {
		seLog.Error(retryErr, "Error updating status for ServiceExport")
	}
}
func getServiceExport(name, namespace string, client client.Client, scheme *runtime.Scheme) (*mcsv1a1.ServiceExport, error) {
	obj, err := client.Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving ServiceExport")
	}

	se := &mcsv1a1.ServiceExport{}

	err = scheme.Convert(obj, se, nil)
	if err != nil {
		return nil, errors.WithMessagef(err, "Error converting %#v to ServiceExport", obj)
	}

	return se, nil
}


*/
func ServiceExportConditionEqual(c1, c2 *mcsv1a1.ServiceExportCondition) bool {
	return c1.Type == c2.Type && c1.Status == c2.Status && reflect.DeepEqual(c1.Reason, c2.Reason) &&
		reflect.DeepEqual(c1.Message, c2.Message)
}
