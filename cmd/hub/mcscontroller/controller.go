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

package main

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/submariner-io/lighthouse/pkg/lhutil"
	"github.com/submariner-io/lighthouse/pkg/mcs"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

const (
	serviceExportFinalizerName = "lighthouse.submariner.io/service-export-finalizer"
)

// ServiceExportReconciler reconciles a ServiceExport object
type ServiceExportReconciler struct {
	Client client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	//	mcsClient apis.ServiceExportInterface
}

// +kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceexports,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceexports/finalizers,verbs=get;update;delete
// +kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceexports/status,verbs=get;update
// +kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceimports,verbs=create;get;list;update;patch;delete

// Reconcile handles an update to a ServiceExport
// By design, the event that triggered reconciliation is not passed to the reconciler
// to ensure actions are based on state alone. This approach is referred to as level-based,
// as opposed to edge-based.
func (r *ServiceExportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	log := r.Log.WithValues("serviceexport", req.NamespacedName)

	// @todo extract and log the real (i.e., source) object name, namespace and cluster?
	log.Info("Reconciling ServiceExport")

	se := &mcsv1a1.ServiceExport{}
	if err := r.Client.Get(ctx, req.NamespacedName, se); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("No ServiceExport found")
		} else {
			// @tdo avoid requeuing for now. Revisit once we see what errors do show.
			log.Error(err, "Error fetching ServiceExport")
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(se, serviceExportFinalizerName) && se.GetDeletionTimestamp() == nil {
		controllerutil.AddFinalizer(se, serviceExportFinalizerName)
		if err := r.Client.Update(ctx, se); err != nil {
			if apierrors.IsConflict(err) { // The SE has been updated since we read it, requeue to retry reconciliation.
				return ctrl.Result{Requeue: true}, nil
			}
			log.Error(err, "Error adding finalizer")
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcile(ctx, lhutil.GetOriginalObjectName(se.ObjectMeta))
	if err != nil {
		return result, err
	}

	if se.GetDeletionTimestamp() != nil { // service export is marked to be deleted
		return r.handleDelete(ctx, se)
	}

	return result, err
}

// reconcile the cluster state.
//
// Reconciliation logic:
// 1. list all relevant exports (i.e., having the same name and namespace labels)
// 2. determine the primary export object
// 3. iterate on all relevant export objects
// 3a. if the object is being deleted (non empty deletion timestamp) - ignore
// 3b. if object conflicts with primary - update export.Status.Condition.Conflict
//		and delete corresponding ServiceImport.
// 3c. otherwise, create/update the corrresponding ServiceImport and set the
//		ServiceExport as owner.
//
// Notes:
// - ensure objects are protected with a finalizer. Since we only receive the
//	 affected object name into the reconciliation loop, we have no way to determine
//	 and retrieve affected ServiceExports (i.e., client.List would not return the
//   deleted object and we can't access its labels). The finalizer is cleared when we
// 	 receive a notification for change in an object scheduled for deletion.
// - ServiceImports are owned by the corresponding ServiceExport and are thus
//	 automatically deleted by k8s when their ServiceExport is deleted.
//
func (r *ServiceExportReconciler) reconcile(ctx context.Context, name types.NamespacedName) (ctrl.Result, error) {
	log := r.Log.WithValues("service", name)
	//1:
	var exportList mcsv1a1.ServiceExportList
	if err := r.Client.List(ctx, &exportList, client.MatchingLabels{"name": name.Name, "namespace": name.Namespace}); err != nil {
		log.Error(err, "unable to list service's service export")
		return ctrl.Result{}, err
	}
	//2:
	primaryEs, err := getPrimaryExportObject(exportList)
	if err != nil {
		return ctrl.Result{}, err
	}
	//3:
	for _, se := range exportList.Items {
		if se.DeletionTimestamp != nil { //3a:
			continue
		}

		exportSpec := &mcs.ExportSpec{}
		err := exportSpec.UnmarshalObjectMeta(&se.ObjectMeta)
		if err != nil {
			log.Info("Failed to unmarshal exportSpec from serviceExport")
			return ctrl.Result{}, err
		}

		err = exportSpec.IsCompatibleWith(primaryEs)
		//3b:
		if err != nil {
			r.updateServiceExportConditions(ctx, &se, mcsv1a1.ServiceExportConflict, corev1.ConditionTrue, "", err.Error())
			r.Client.Status().Update(ctx, &se)
			//r.mcsClient.UpdateStatus(ctx, &se, metav1.UpdateOptions{})
			err = r.deleteImport(ctx, &se)
			if err != nil {
				return ctrl.Result{}, err
			}
		} else { //3c:
			r.updateServiceExportConditions(ctx, &se, mcsv1a1.ServiceExportValid, corev1.ConditionTrue, "", "")
			r.Client.Status().Update(ctx, &se)
			//se, err := r.mcsClient.UpdateStatus(ctx, &se, metav1.UpdateOptions{})
			if err != nil {
				return ctrl.Result{}, err
			}
			r.ensureImportFor(ctx, &se)
		}
	}
	return ctrl.Result{}, nil
}

// getPrimaryExportObject finds the Primary ExportSpec from ServiceExportList.
// Retruns the ExportSpec object that determined as the primary one from
// the given ServiceExportList according to the conflict resolution specification set in the
// KEP, that is implemented in IsPreferredOver method of ExportSpec.
func getPrimaryExportObject(exportList mcsv1a1.ServiceExportList) (*mcs.ExportSpec, error) {
	var primary *mcs.ExportSpec
	for i, se := range exportList.Items {
		exportSpec := &mcs.ExportSpec{}
		err := exportSpec.UnmarshalObjectMeta(&se.ObjectMeta)
		if err != nil {
			return nil, err
		}
		if i == 0 || exportSpec.IsPreferredOver(primary) {
			primary = exportSpec
		}
	}
	return primary, nil
}

// create or update the ServiceImport corresponding to the provided export.
func (r *ServiceExportReconciler) ensureImportFor(ctx context.Context, se *mcsv1a1.ServiceExport) error {
	es := &mcs.ExportSpec{}

	err := es.UnmarshalObjectMeta(&se.ObjectMeta)
	if err != nil {
		return err
	}
	namespacedName := types.NamespacedName{
		Namespace: es.Name,
		Name:      es.Namespace,
	}
	log := r.Log.WithValues("service", namespacedName.Name)

	si := mcsv1a1.ServiceImport{}
	err = r.Client.Get(ctx, namespacedName, &si)
	expected := mcsv1a1.ServiceImport{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      es.Name,
		},
		Spec: mcsv1a1.ServiceImportSpec{
			Type:  es.Service.Type,
			Ports: es.Service.Ports,
		},
	}
	if apierrors.IsNotFound(err) {
		err = r.Client.Create(ctx, &expected)
		if err != nil {
			log.Error(err, "unable to create the needed Service Import")
			return err
		}
	} else if err == nil {
		err = r.updateSi(ctx, &si, &expected, log)
		if err != nil {
			return err
		}
	} else {
		log.Error(err, "Failed to get the needed service import")
		return err
	}
	return nil
}

func (r *ServiceExportReconciler) updateSi(ctx context.Context, si *mcsv1a1.ServiceImport, expected *mcsv1a1.ServiceImport, log logr.Logger) error {
	//not sure it's really needed to check name and namespace - I got the si from the Get according to those fields
	//however its a general func, maybe should be "safe" for other usages(?)
	if si.Name == expected.Name && si.Namespace == expected.Namespace {
		si.Spec.Type = expected.Spec.Type
		//maybe add a function that change the ports more carefully, without overwriting
		si.Spec.Ports = expected.Spec.Ports
		err := r.Client.Update(ctx, si)
		if err != nil {
			log.Error(err, "unable to update the needed Service Import")
			return err
		}
	}
	return nil
}

// delete the corresponding serviceImport of the given serviceExport if exists.
func (r *ServiceExportReconciler) deleteImport(ctx context.Context, se *mcsv1a1.ServiceExport) error {
	es := &mcs.ExportSpec{}

	err := es.UnmarshalObjectMeta(&se.ObjectMeta)
	if err != nil {
		return err
	}
	namespacedName := types.NamespacedName{
		Namespace: es.Name,
		Name:      es.Namespace,
	}
	log := r.Log.WithValues("service", namespacedName.Name)

	si := mcsv1a1.ServiceImport{}
	err = r.Client.Get(ctx, namespacedName, &si)
	si = mcsv1a1.ServiceImport{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      es.Name,
		},
		Spec: mcsv1a1.ServiceImportSpec{
			Type:  es.Service.Type,
			Ports: es.Service.Ports,
		},
	}
	if apierrors.IsNotFound(err) {
		//the serviceImport dosen't exists - don't do anything
		return nil
	} else if err == nil {
		//delete the service Import
		r.Client.Delete(ctx, &si)
	} else {
		log.Error(err, "Failed to get the service Import to delete")
		return err
	}
	return nil
}

//update the condition field under serviceExport status
//based on the already existing method in the agent - might move to lhutil
func (r *ServiceExportReconciler) updateServiceExportConditions(ctx context.Context, se *mcsv1a1.ServiceExport,
	conditionType mcsv1a1.ServiceExportConditionType, status corev1.ConditionStatus, reason string, msg string) error {
	log := r.Log.WithValues("name", se.Namespace+"/"+se.Name)
	now := metav1.Now()
	exportCondition := mcsv1a1.ServiceExportCondition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: &now,
		Reason:             (*string)(&reason),
		Message:            &msg,
	}

	numCond := len(se.Status.Conditions)
	if numCond > 0 && lhutil.ServiceExportConditionEqual(&se.Status.Conditions[numCond-1], &exportCondition) {
		lastCond := se.Status.Conditions[numCond-1]
		log.Info("Last ServiceExportCondition equal - not updating status",
			"condition", lastCond)
		return nil
	}

	if numCond >= lhutil.MaxExportStatusConditions {
		copy(se.Status.Conditions[0:], se.Status.Conditions[1:])
		se.Status.Conditions = se.Status.Conditions[:lhutil.MaxExportStatusConditions]
		se.Status.Conditions[lhutil.MaxExportStatusConditions-1] = exportCondition
	} else {
		se.Status.Conditions = append(se.Status.Conditions, exportCondition)
	}

	return nil
}

// allow the object to be deleted by removing the finalizer.
// Once all finalizers have been removed, the ServiceExport object and its
// owned ServiceImport object will be deleted.
//
// Note: ownership will need to be reconsidered if merging imports
func (r *ServiceExportReconciler) handleDelete(ctx context.Context, se *mcsv1a1.ServiceExport) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(se, serviceExportFinalizerName) {
		// @todo: add the NamespacedName of the ServiceExport to be compatible with Reconcile?
		log := r.Log.WithValues("service", lhutil.GetOriginalObjectName(se.ObjectMeta)).
			WithValues("cluster", lhutil.GetOriginalObjectCluster(se.ObjectMeta))

		log.Info("Removing service export finalizer")

		// clear the finalizer
		controllerutil.RemoveFinalizer(se, serviceExportFinalizerName)
		if err := r.Client.Update(ctx, se); err != nil {
			// @todo handle version mismatch error?
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconciler with the manager.
func (r *ServiceExportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcsv1a1.ServiceExport{}).
		Owns(&mcsv1a1.ServiceImport{}).
		Complete(r)
}
