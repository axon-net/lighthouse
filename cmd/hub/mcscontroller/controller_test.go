package mcscontroller_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/submariner-io/lighthouse/cmd/hub/mcscontroller"
	lhconst "github.com/submariner-io/lighthouse/pkg/constants"
	"github.com/submariner-io/lighthouse/pkg/lhutil"
	"github.com/submariner-io/lighthouse/pkg/mcs"
)

const (
	serviceName = "service"
	serviceNS   = "namespace"
	brokerNS    = "submariner-k8s-broker"
	cluster1    = "c1"
	cluster2    = "c2"
)

var (
	now     = metav1.NewTime(time.Now())
	service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: serviceNS,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "1.2.3.4",
			Ports: []corev1.ServicePort{{
				Name: "http",
				Port: int32(80),
			}},
		},
	}
	export = &mcsv1a1.ServiceExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: serviceNS,
			Labels: map[string]string{
				lhconst.LabelSourceNamespace:      serviceNS,
				lhconst.LighthouseLabelSourceName: serviceName,
			},
			CreationTimestamp: now,
		},
		Status: mcsv1a1.ServiceExportStatus{
			Conditions: []mcsv1a1.ServiceExportCondition{{
				Type:               mcsv1a1.ServiceExportValid,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: &now,
			}},
		},
	}
)

func TestCreatesImport(t *testing.T) {
	se, err := prepareExport(*export, cluster1)
	if err != nil {
		t.Error(err)
	}

	preloadedObjs := []runtime.Object{se}
	r := mcscontroller.ServiceExportReconciler{
		Client: getClient(preloadedObjs),
		Log:    newLogger(t, false),
		Scheme: getScheme(),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      se.GetName(),
			Namespace: se.GetNamespace(),
		}}

	result, err := r.Reconcile(context.TODO(), req)

	if err != nil {
		t.Error(err)
	}
	if result.Requeue {
		t.Error("unexpected requeue")
	}

	if err = r.Client.Get(context.TODO(), req.NamespacedName, se); err != nil {
		t.Error(err)
	}
	if hasCondition(*se, mcsv1a1.ServiceExportConflict, corev1.ConditionTrue) {
		t.Error("unexpected conflict condition")
	}

	si := mcsv1a1.ServiceImport{}
	if err = r.Client.Get(context.TODO(), req.NamespacedName, &si); err != nil {
		t.Error(err)
	}
	validateImportForExport(si, *se)
}

func TestCreatesImportPerEachExport(t *testing.T) {
}

func TestDeletingExportDeletesImport(t *testing.T) {
}

func TestImportUnchangedWhenUpdatingNonGlobalExportProperties(t *testing.T) {
}

func TestImportUpdatedWithExportGlobalProperties(t *testing.T) {
}

func TestImportForOnlyNonConflictingExport(t *testing.T) {
}

// prepare a cluster-specific export based on the given service export
func prepareExport(se mcsv1a1.ServiceExport, cluster string) (*mcsv1a1.ServiceExport, error) {
	exp := se.DeepCopy()
	exp.Labels[lhconst.LighthouseLabelSourceCluster] = cluster
	spec, err := mcs.NewExportSpec(service, exp, cluster)
	if err != nil {
		return nil, err
	}
	if err := spec.MarshalObjectMeta(&exp.ObjectMeta); err != nil {
		return nil, err
	}
	exp.Name = lhutil.GenerateObjectName(serviceName, serviceNS, cluster1)
	exp.Namespace = brokerNS
	return exp, nil
}

// validate the attributes of the import are appropriate for the given export
func validateImportForExport(si mcsv1a1.ServiceImport, se mcsv1a1.ServiceExport) error {
	var expSpec mcs.ExportSpec
	if err := expSpec.UnmarshalObjectMeta(&se.ObjectMeta); err != nil {
		return err
	}

	if si.Spec.Type != expSpec.Service.Type {
		return fmt.Errorf("unexpected service type. want %v got %v", expSpec.Service.Type, si.Spec.Type)
	}
	// @todo for now assume single Port
	if len(si.Spec.Ports) != len(service.Spec.Ports) {
		return fmt.Errorf("unexpected service port count, want %d got %d", len(service.Spec.Ports), len(si.Spec.Ports))
	} else if !reflect.DeepEqual(si.Spec.Ports, service.Spec.Ports) {
		return fmt.Errorf("unexpected service port list, want %v got %v", service.Spec.Ports, si.Spec.Ports)
	}

	if len(si.Status.Clusters) != 1 {
		return fmt.Errorf("unexpected cluster list length %d", len(si.Status.Clusters))
	} else if si.Status.Clusters[0].Cluster != expSpec.ClusterID {
		return fmt.Errorf("unexpected cluster want %s got %s", expSpec.ClusterID, si.Status.Clusters[0].Cluster)
	}

	labels := si.GetLabels()
	if labels[lhconst.LabelSourceNamespace] != expSpec.Namespace ||
		labels[lhconst.LighthouseLabelSourceName] != expSpec.Name ||
		labels[lhconst.LighthouseLabelSourceCluster] != expSpec.ClusterID {
		return fmt.Errorf("mismatching labels want %v got %v", expSpec, labels)
	}
	// @todo session affinity

	return nil
}

func hasCondition(se mcsv1a1.ServiceExport, ct mcsv1a1.ServiceExportConditionType,
	status corev1.ConditionStatus) bool {

	// @todo if multiple conditions of the same type exist, ensure ours has the latest timestamp?
	for _, cond := range se.Status.Conditions {
		if cond.Type == ct && cond.Status == status {
			return true
		}
	}
	return false
}

// return a scheme with the default k8s and mcs objects
func getScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(mcsv1a1.AddToScheme(scheme))
	return scheme
}

// generate a fake client with preloaded objects
func getClient(objs []runtime.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(getScheme()).WithRuntimeObjects(objs...).Build()
}

// satisfy the logr.Logger interface with logging to the test's output
type logger struct {
	enabled bool
	t       *testing.T
	name    string
	kv      map[string]interface{}
}

var _ logr.Logger = &logger{}

func newLogger(t *testing.T, enabled bool) *logger {
	return &logger{
		enabled: enabled,
		t:       t,
		kv:      make(map[string]interface{}),
	}
}

func (l logger) Enabled() bool {
	return l.enabled
}

func (l logger) Error(e error, msg string, keysAndValues ...interface{}) {
	if l.enabled {
		args := []interface{}{e.Error(), msg, keysAndValues, l.kv}
		l.t.Error(args...)
	}
}

func (l logger) Info(msg string, keysAndValues ...interface{}) {
	if l.enabled {
		args := []interface{}{msg, keysAndValues, l.kv}
		l.t.Log(args...)
	}
}

func (l *logger) V(level int) logr.Logger {
	return l
}

func (l *logger) WithValues(keysAndValues ...interface{}) logr.Logger {
	if len(keysAndValues)%2 != 0 {
		panic(keysAndValues)
	}

	for i := 0; i < len(keysAndValues); i += 2 {
		l.kv[keysAndValues[i].(string)] = keysAndValues[i+1]
	}
	return l
}

func (l *logger) WithName(name string) logr.Logger {
	l.name = name
	return l
}
