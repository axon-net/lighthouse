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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

const (
	serviceExportFinalizer = "lighthouse.submariner.io/service-export-finalizer"
)

// ServiceExportReconciler reconciles a ServiceExport object
type ServiceExportReconciler struct {
	Client client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceexports,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceexports/finalizers,verbs=get;update
// +kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceexports/status,verbs=get;update
// +kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceimports,verbs=create;get;list;update;patch

// Reconcile handles an update to a ServiceExport
// By design, the event that triggered reconciliation is not passed to the reconciler
// to ensure actions are based on state alone. This approach is referred to as level-based,
// as opposed to edge-based.
func (r *ServiceExportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// @todo extract the real (i.e., source) object name, namespace and cluster
	log := r.Log.WithValues("serviceexport", req.NamespacedName)
	log.Info("reconciling ServiceExport")

	se := mcsv1a1.ServiceExport{}
	if err := r.Client.Get(ctx, req.NamespacedName, &se); err != nil {
		if errors.IsNotFound(err) {
			log.Info("no ServiceExport found")
		} else {
			log.Error(err, "error fetching ServiceExport")
		}
		return ctrl.Result{}, nil
	}

	if se.GetDeletionTimestamp() != nil { // service export is marked to be deleted
		return r.handleDelete(ctx, &se)
	}

	return r.handleUpdate(ctx, &se)
}

// handle create and update of service export
func (r *ServiceExportReconciler) handleUpdate(ctx context.Context, se *mcsv1a1.ServiceExport) (ctrl.Result, error) {
	// @todo extract the real (i.e., source) object name, namespace and cluster
	log := r.Log.WithValues("serviceexport", types.NamespacedName{se.Namespace, se.Name})

	if !controllerutil.ContainsFinalizer(se, serviceExportFinalizer) {
		controllerutil.AddFinalizer(se, serviceExportFinalizer)
		if err := r.Client.Update(ctx, se); err != nil {
			log.Error(err, "error adding finalizer")
			return ctrl.Result{}, err
		}
	}

	// @todo
	// fetch all relevant ServiceExports
	// Compute primary and call updateImports with new primary
	// @todo avoid recomputing conflicts and service imports if primary hasn't changed
	//   (marked by annotation or label). In that case only call compute single ServiceImport
	return ctrl.Result{}, nil
}

func (r *ServiceExportReconciler) handleDelete(ctx context.Context, se *mcsv1a1.ServiceExport) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(se, serviceExportFinalizer) {
		// @todo extract the real (i.e., source) object name, namespace and cluster
		log := r.Log.WithValues("serviceexport", types.NamespacedName{se.Namespace, se.Name})
		log.Info("removing service export")

		// @todo:
		// fetch all relevant service exports
		// if removing the "primary" SE, recompute primary and call updateImports with new primary
		// @todo: avoid recomputing conflicts if the spec of the previous and new primaries is identical?

		// clear the finalizer (once all finalizers have been removed, the ServiceExport
		// object and its owned ServiceImport object will be deleted).
		// Note: ownership will need to be reconsidered if merging imports
		controllerutil.RemoveFinalizer(se, serviceExportFinalizer)
		if err := r.Client.Update(ctx, se); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *ServiceExportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcsv1a1.ServiceExport{}).
		Owns(&mcsv1a1.ServiceImport{}).
		Complete(r)
}
