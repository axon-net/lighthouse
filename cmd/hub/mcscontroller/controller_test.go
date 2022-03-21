package mcscontroller_test

import (
	"context"
	"testing"

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
	serviceName = "svc"
	serviceNS   = "svc-ns"
	brokerNS    = "submariner-broker"
	cluster1    = "c1"
	cluster2    = "c2"
)

var (
	service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: serviceNS,
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
		},
	}
)

func prepareServiceExport() (*mcsv1a1.ServiceExport, *mcs.ExportSpec, error) {
	exp := export.DeepCopy()
	exp.Labels[lhconst.LighthouseLabelSourceCluster] = cluster1
	es, err := mcs.NewExportSpec(service, exp, cluster1)
	return exp, es, err
}

func TestImportGenerated(t *testing.T) {
	// @todo refactor into a prepareServiceExport function
	exp1, es, err := prepareServiceExport()
	if err != nil {
		t.Error(err)
	}
	err = es.MarshalObjectMeta(&exp1.ObjectMeta)
	if err != nil {
		t.Error(err)
	}

	preloadedObjects := []runtime.Object{service, exp1}
	ser := mcscontroller.ServiceExportReconciler{
		Client: getClient(preloadedObjects),
		Log:    newLogger(t, false),
		Scheme: getScheme(),
	}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      exp1.GetName(),
			Namespace: exp1.GetNamespace(),
		}}

	// mimics the changing in the names that happens in the hub
	//TO PR - make sure it suppose to be after taking the names to the req
	exp1.Name = lhutil.GenerateObjectName(serviceName, serviceNS, cluster1)
	exp1.Namespace = brokerNS

	result, err := ser.Reconcile(context.TODO(), req)

	if err != nil {
		t.Error(err)
	}
	if result.Requeue {
		t.Error("unexpected requeue")
	}

	si := mcsv1a1.ServiceImport{}
	err = ser.Client.Get(context.TODO(), req.NamespacedName, &si)

	if err != nil {
		t.Error(err)
	}
	// @todo validate the ServiceImport properties
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

// satisfy the logr.Logger interface with a nil logger
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
