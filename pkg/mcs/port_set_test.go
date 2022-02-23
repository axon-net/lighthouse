package mcs_test

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/submariner-io/lighthouse/pkg/mcs"
)

func TestEmptyPortSetAreEqual(t *testing.T) {
	ps := mcs.NewPortSet()
	if ps.Len() != 0 {
		t.Error("Len() != 0")
	}
	if !ps.Equals(mcs.NewPortSet()) {
		t.Error("empty PortSets not equal")
	}
}

func TestCanCreatePortSetFromService(t *testing.T) {
	testcases := []struct {
		input []corev1.ServicePort
	}{
		{input: []corev1.ServicePort{{Port: 80}}},
		{input: []corev1.ServicePort{{Port: 80}, {Name: "https", Port: 443}}},
		{input: []corev1.ServicePort{{Name: "http", Port: 80}, {Name: "https", Port: 443}}},
	}

	for i, tc := range testcases {
		ps, err := mcs.NewPortSetFromServicePorts(tc.input)
		if err != nil {
			t.Fatalf("%d: %s", i, err)
		} else if ps.Len() != len(tc.input) {
			t.Fatalf("%d: Len() returned %d expected %d", i, ps.Len(), len(tc.input))
		}
	}
}

func TestPortSetStringRoundTrip(t *testing.T) {
	testcases := []struct {
		input []corev1.ServicePort
	}{
		{input: []corev1.ServicePort{{Port: 80}}},
		{input: []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP}}},
		{input: []corev1.ServicePort{{Name: "http", Port: 80}}},
		{input: []corev1.ServicePort{{Name: "http", Port: 80}, {Name: "https", Port: 443}}},
		{input: []corev1.ServicePort{{Port: 80}, {Name: "https", Port: 443}}},
	}

	for i, tc := range testcases {
		original, err := mcs.NewPortSetFromServicePorts(tc.input)
		if err != nil {
			t.Fatalf("input %d: %s", i, err)
		}
		meta := &metav1.ObjectMeta{
			Annotations: make(map[string]string),
		}
		if err := original.Annotate(meta); err != nil {
			t.Fatalf("annotate %d: %s", i, err)
		}
		derived, err := mcs.NewPortSetFromObjectMeta(meta)
		if err != nil {
			t.Fatalf("restore %d: %s", i, err)
		}
		if derived.Len() != original.Len() {
			t.Errorf("len %d wanted: %d got %d", i, original.Len(), derived.Len())
		}
		if !derived.Equals(original) {
			fmt.Println(*original, *derived)
			t.Fatalf("equals %d", i)
		}
	}
}

func TestPortSetConflicts(t *testing.T) {
	testcases := []struct {
		base     []corev1.ServicePort
		set      []corev1.ServicePort
		conflict bool
	}{
		{base: []corev1.ServicePort{{Port: 80}}, set: []corev1.ServicePort{{Port: 80}}, conflict: false},
		{
			base:     []corev1.ServicePort{{Name: "http", Port: 80}},
			set:      []corev1.ServicePort{{Name: "http", Port: 80}},
			conflict: false,
		},
		{
			base:     []corev1.ServicePort{{Port: 80}, {Name: "https", Port: 443}},
			set:      []corev1.ServicePort{{Name: "https", Port: 443}},
			conflict: false,
		},
		{base: []corev1.ServicePort{{Port: 80}}, set: []corev1.ServicePort{{}}, conflict: true},
		{
			base:     []corev1.ServicePort{{Port: 80}, {Name: "https", Port: 443}},
			set:      []corev1.ServicePort{{Port: 443}},
			conflict: true,
		},
		{
			base:     []corev1.ServicePort{{Port: 443, Protocol: corev1.ProtocolTCP}},
			set:      []corev1.ServicePort{{Port: 443, Protocol: corev1.ProtocolUDP}},
			conflict: true,
		},
		{
			base:     []corev1.ServicePort{{Name: "smtp", Port: 25}, {Name: "https", Port: 443}},
			set:      []corev1.ServicePort{{Name: "ssh", Port: 22}, {Name: "https", Port: 8443}},
			conflict: true,
		},
	}

	for i, tc := range testcases {
		base, err := mcs.NewPortSetFromServicePorts(tc.base)
		if err != nil {
			t.Fatalf("base %d: %s", i, err)
		}
		set, err := mcs.NewPortSetFromServicePorts(tc.set)
		if err != nil {
			t.Fatalf("set %d: %s", i, err)
		}
		if set.IsConflictingWith(base) != tc.conflict {
			t.Fatalf("conflict %d: wanted %t", i, tc.conflict)
		}
	}
}

func TestPortSetIsOrderInsensitive(t *testing.T) {
	ports := []corev1.ServicePort{{Name: "http", Port: 80}, {Name: "https", Port: 443}}
	original, _ := mcs.NewPortSetFromServicePorts(ports)
	swapped, _ := mcs.NewPortSetFromServicePorts(swap(ports))

	if !original.Equals(swapped) {
		t.Fatalf("swap not equal")
	}
}

func TestCanMergeWhenSubset(t *testing.T) {
	// {1, [then add 4]}, {1,2,3} => {1,2,3,[4]}

}

func TestMergeUnion(t *testing.T) {
	set1 := []corev1.ServicePort{
		{Name: "smtp", Port: 25},
		{Name: "https", Port: 443},
	}
	set2 := []corev1.ServicePort{
		{Name: "ssh", Port: 22},
		{Name: "https", Port: 443},
	}
	merged := []corev1.ServicePort{
		{Name: "ssh", Port: 22},
		{Name: "smtp", Port: 25},
		{Name: "https", Port: 443},
	}

	s1, _ := mcs.NewPortSetFromServicePorts(set1)
	s2, _ := mcs.NewPortSetFromServicePorts(set2)
	s3, _ := mcs.NewPortSetFromServicePorts(merged)

	union, err := s1.Merge(s2)
	if err != nil || !union.Equals(s3) {
		t.Fatalf("err: %v wanted: %v got %v", err, s3, union)
	}
}

func swap(ports []corev1.ServicePort) []corev1.ServicePort {
	s := ports
	if len(s) > 1 {
		s[0], s[1] = s[1], s[0]
	}
	return s
}
