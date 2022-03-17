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
)

const (
	serviceName = "svc"
	serviceNS   = "svc-ns"
	brokerNS    = "submariner-broker"
	cluster1    = "c1"
	cluster2    = "c2"
)

var (
	scheme  = runtime.NewScheme()
	service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: serviceNS,
		},
	}
	export = &mcsv1a1.ServiceExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "placeholder",
			Namespace: brokerNS,
			Labels: map[string]string{
				lhconst.LabelSourceNamespace:      serviceNS,
				lhconst.LighthouseLabelSourceName: serviceName,
			},
		},
	}
)

func TestImportGenerated(t *testing.T) {
	exp1 := export.DeepCopy()
	exp1.Name = lhutil.GenerateObjectName(serviceName, serviceNS, cluster1)
	exp1.Labels[lhconst.LighthouseLabelSourceCluster] = cluster1

	preloadedObjects := []runtime.Object{service, exp1}
	ser := mcscontroller.ServiceExportReconciler{
		Client: getClient(preloadedObjects),
		Log:    &nilLog{},
		Scheme: getScheme(),
	}

	_, err := ser.Reconcile(context.TODO(), reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      exp1.GetName(),
			Namespace: exp1.GetNamespace(),
		}})
	if err != nil {
		t.Error(err)
	}

	si := mcsv1a1.ServiceImport{}
	err = ser.Client.Get(context.TODO(), types.NamespacedName{
		Name:      exp1.GetName(),
		Namespace: exp1.GetNamespace(),
	}, &si)

	if err != nil {
		t.Error(err)
	}
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(mcsv1a1.AddToScheme(scheme))
}

func getScheme() *runtime.Scheme {
	return scheme
}

func getClient(objs []runtime.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(getScheme()).WithRuntimeObjects(objs...).Build()
}

type nilLog struct{}

func (nilLog) Enabled() bool                                           { return true }
func (nilLog) Error(e error, msg string, keysAndValues ...interface{}) {}
func (nilLog) Info(msg string, keysAndValues ...interface{})           {}
func (dn *nilLog) V(level int) logr.Logger                             { return dn }
func (dn *nilLog) WithValues(keysAndValues ...interface{}) logr.Logger { return dn }
func (dn *nilLog) WithName(name string) logr.Logger                    { return dn }
