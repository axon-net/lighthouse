package mcscontroller_test

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	timeout int32 = 10
	service       = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: serviceNS,
		},
		Spec: corev1.ServiceSpec{
			Type:            corev1.ServiceTypeClusterIP,
			SessionAffinity: corev1.ServiceAffinityClientIP,
			SessionAffinityConfig: &corev1.SessionAffinityConfig{
				ClientIP: &corev1.ClientIPConfig{
					TimeoutSeconds: &timeout,
				},
			},
			Ports: []corev1.ServicePort{
				{Port: 80, Name: "http", Protocol: corev1.ProtocolTCP},
			},
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

func prepareServiceExport(t *testing.T) (*mcsv1a1.ServiceExport, *mcs.ExportSpec) {
	exp := export.DeepCopy()
	exp.Labels[lhconst.LighthouseLabelSourceCluster] = cluster1
	es, err := mcs.NewExportSpec(service, exp, cluster1)
	if err != nil {
		t.Error(err)
	}
	err = es.MarshalObjectMeta(&exp.ObjectMeta)
	if err != nil {
		t.Error(err)
	}
	return exp, es
}

func TestImportGenerated(t *testing.T) {
	exp, es := prepareServiceExport(t)

	preloadedObjects := []runtime.Object{service, exp}
	ser := mcscontroller.ServiceExportReconciler{
		Client: getClient(preloadedObjects),
		Log:    newLogger(t, false),
		Scheme: getScheme(),
	}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      exp.GetName(),
			Namespace: exp.GetNamespace(),
		}}

	// mimics the changing in the names that happens in the hub
	//TO PR - make sure it suppose to be after taking the names to the req. maybe don't need it?
	exp.Name = lhutil.GenerateObjectName(serviceName, serviceNS, cluster1)
	exp.Namespace = brokerNS

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
	assertions := require.New(t)
	compareSi(&si, exp, es, assertions)
}

func TestImportFromExport(t *testing.T) {
	exp, es := prepareServiceExport(t)

	preloadedObjects := []runtime.Object{service}
	ser := mcscontroller.ServiceExportReconciler{
		Client: getClient(preloadedObjects),
		Log:    newLogger(t, false),
		Scheme: getScheme(),
	}
	ser.Client.Create(context.TODO(), exp)
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      exp.GetName(),
			Namespace: exp.GetNamespace(),
		}}

	// mimics the changing in the names that happens in the hub
	//TO PR - make sure it suppose to be after taking the names to the req. maybe don't need it?
	exp.Name = lhutil.GenerateObjectName(serviceName, serviceNS, cluster1)
	exp.Namespace = brokerNS

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
	assertions := require.New(t)
	compareSi(&si, exp, es, assertions)
}

//FAILS CLUSTER FIELD - make sure how to test it and enter this field.
func Test2ImportsFromExports(t *testing.T) {
	exp1, es1 := prepareServiceExport(t)

	preloadedObjects := []runtime.Object{service}
	ser := mcscontroller.ServiceExportReconciler{
		Client: getClient(preloadedObjects),
		Log:    newLogger(t, false),
		Scheme: getScheme(),
	}
	ser.Client.Create(context.TODO(), exp1)
	req1 := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      exp1.GetName(),
			Namespace: exp1.GetNamespace(),
		}}

	// mimics the changing in the names that happens in the hub
	//TO PR - make sure it suppose to be after taking the names to the req. maybe don't need it?
	exp1.Name = lhutil.GenerateObjectName(serviceName, serviceNS, cluster1)
	exp1.Namespace = brokerNS

	result, err := ser.Reconcile(context.TODO(), req1)

	if err != nil {
		t.Error(err)
	}
	if result.Requeue {
		t.Error("unexpected requeue")
	}

	verfiySi(ser, t, req1, exp1, es1)

	exp2, es2 := prepareServiceExport(t)
	exp2.Labels[lhconst.LighthouseLabelSourceCluster] = cluster2
	ser.Client.Create(context.TODO(), exp2)
	req2 := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      exp2.GetName(),
			Namespace: exp2.GetNamespace(),
		}}

	// mimics the changing in the names that happens in the hub
	//TO PR - make sure it suppose to be after taking the names to the req. maybe don't need it?
	exp2.Name = lhutil.GenerateObjectName(serviceName, serviceNS, cluster2)
	exp2.Namespace = brokerNS

	result, err = ser.Reconcile(context.TODO(), req2)

	if err != nil {
		t.Error(err)
	}
	if result.Requeue {
		t.Error("unexpected requeue")
	}
	verfiySi(ser, t, req2, exp2, es2)
}

// FAILS - The service import is not deleted after deleting the service export
// not sure why - The Reconcil function is finishing eraly since it sees that the se object is deleted
// I think it should delete the service import too since its owner is the serviceExport that was deleted.
func TestCreateAndDeleteExport(t *testing.T) {
	assertions := require.New(t)
	exp1, es1 := prepareServiceExport(t)
	preloadedObjects := []runtime.Object{service}
	ser := mcscontroller.ServiceExportReconciler{
		Client: getClient(preloadedObjects),
		Log:    newLogger(t, false),
		Scheme: getScheme(),
	}
	ser.Client.Create(context.TODO(), exp1)
	req1 := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      exp1.GetName(),
			Namespace: exp1.GetNamespace(),
		}}
	result, err := ser.Reconcile(context.TODO(), req1)

	if err != nil {
		t.Error(err)
	}
	if result.Requeue {
		t.Error("unexpected requeue")
	}

	verfiySi(ser, t, req1, exp1, es1)

	//delete the export - and check that the import was also deleted:
	ser.Client.Delete(context.TODO(), exp1)
	result, err = ser.Reconcile(context.TODO(), req1)

	if err != nil {
		t.Error(err)
	}
	if result.Requeue {
		t.Error("unexpected requeue")
	}
	si := mcsv1a1.ServiceImport{}
	err = ser.Client.Get(context.TODO(), req1.NamespacedName, &si)
	assertions.True(apierrors.IsNotFound(err))
}

func verfiySi(ser mcscontroller.ServiceExportReconciler, t *testing.T, req reconcile.Request,
	exp *mcsv1a1.ServiceExport, es *mcs.ExportSpec) {
	si := mcsv1a1.ServiceImport{}
	err := ser.Client.Get(context.TODO(), req.NamespacedName, &si)

	if err != nil {
		t.Error(err)
	}
	assertions := require.New(t)
	compareSi(&si, exp, es, assertions)
}

// To ask Etai - is those fields check ok? are the name and namespace should be in this format?
// compare two service imports field and raise error if they are not with identical values
func compareSi(si *mcsv1a1.ServiceImport, se *mcsv1a1.ServiceExport, es *mcs.ExportSpec, assertions *require.Assertions) {
	assertions.Equal(si.ObjectMeta.Name, lhutil.GetOriginalObjectName(se.ObjectMeta).Name)
	assertions.Equal(si.ObjectMeta.Namespace, lhutil.GetOriginalObjectName(se.ObjectMeta).Namespace)
	assertions.Equal(si.ObjectMeta.CreationTimestamp, se.CreationTimestamp)
	comparePorts(si.Spec.Ports, es.Service.Ports, assertions)
	assertions.Equal(si.Spec.Type, es.Service.Type)
	assertions.Equal(si.Status.Clusters[0].Cluster, lhutil.GetOriginalObjectCluster(se.ObjectMeta))
}

// compare two ports arrays and raise error if they are not with identical values
func comparePorts(ports1 []mcsv1a1.ServicePort, ports2 []mcsv1a1.ServicePort, assertions *require.Assertions) {
	assertions.Equal(len(ports1), len(ports2))
	for i, port := range ports1 {
		assertions.Equal(port, ports2[i])
	}
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
