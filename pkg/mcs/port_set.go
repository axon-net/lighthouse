package mcs

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

const (
	LIGHTHOUSE_PORT_KEY                   = "lighthouse.submariner.io/port"
	LIGHTHOUSE_PORT_KEY_NAMED_FORMAT      = LIGHTHOUSE_PORT_KEY + ".%s"
	LIGHTHOUSE_PORT_VALUE_FORMAT_PORT     = "%d"
	LIGHTHOUSE_PORT_VALUE_FORMAT_PROTOCOL = LIGHTHOUSE_PORT_VALUE_FORMAT_PORT + "-%s"
	LIGHTHOUSE_PORT_VALUE_FORMAT_FULL     = LIGHTHOUSE_PORT_VALUE_FORMAT_PROTOCOL + "-%s"
)

// PortSet encapsulates Servie ports used.
type PortSet struct {
	ports []mcsv1a1.ServicePort
}

// NewPortSet creates an empty PortSet.
func NewPortSet() *PortSet {
	return &PortSet{}
}

// NewPortSetFromServicePorts creates a PortSet populated from the ports defined
// in v1.Service.Spec. Returns an error when the specification is invalid.
func NewPortSetFromServicePorts(ports []corev1.ServicePort) (*PortSet, error) {
	ps := NewPortSet()

	for _, port := range ports {
		ps.ports = append(ps.ports, mcsv1a1.ServicePort{
			Name:        port.Name,
			Protocol:    port.Protocol,
			AppProtocol: port.AppProtocol,
			Port:        port.Port,
		})
	}
	return ps, nil
}

// NewPortSetFromObjectMeta creates a PortSet from the annotations defined
// on the object's metadata. The format is parsed in accordance to the way
// ports are encoded in Annotate(). Returns nil error and empty PortSet if
// no port specification are found. Returns non-nil error on any parse errors.
func NewPortSetFromObjectMeta(objmeta *metav1.ObjectMeta) (*PortSet, error) {
	ps := NewPortSet()

	for key, value := range objmeta.Annotations {
		port, err := decodePortFromAnnotation(key, value)
		if err != nil {
			return nil, err
		}
		ps.ports = append(ps.ports, *port)
	}
	return ps, nil
}

// Len returns the count of ports in the PortSet.
func (ps *PortSet) Len() int {
	if ps == nil {
		return 0
	}
	return len(ps.ports)
}

// Annotate encodes the PortSet into the object's metadata. The formatting
// is done by encodePortAsAnnotation.
func (ps *PortSet) Annotate(objmeta *metav1.ObjectMeta) error {
	annotations, err := ps.AsAnnotations()
	if err != nil {
		return err
	}

	if objmeta.Annotations == nil {
		objmeta.Annotations = make(map[string]string, len(annotations))
	}

	for key, value := range annotations {
		objmeta.Annotations[key] = value
	}
	return nil
}

// AsAnnotations returns the needed annotations to encode the entire PortSet
func (ps *PortSet) AsAnnotations() (map[string]string, error) {
	annotations := make(map[string]string, len(ps.ports))

	for _, p := range ps.ports {
		key, value, err := encodePortAsAnnotation(&p)
		if err != nil {
			// @TODO consider if we want to encode and return a subset of
			// the ports which were successfully encoded or fail all
			return nil, err
		}
		annotations[key] = value
	}

	return annotations, nil
}

const (
	validateOnlySharedPorts = false
	validateAllPorts        = true
)

// Equal returns true when the PortSets are semantically identical
func (ps *PortSet) Equals(rhs *PortSet) bool {
	return equals(ps, rhs, validateAllPorts)
}

// IsCompatibleWith determines if the PortSet parameter is compatible with
// the current set of ports available in the PortSet.
func (ps *PortSet) IsCompatibleWith(rhs *PortSet) bool {
	return !ps.IsConflictingWith(rhs)
}

// IsConflictingWith determines if the PortSet parameter has any ports
// in conflict with the stored ports. Ports not shared by the sets are
// ignored.
func (ps *PortSet) IsConflictingWith(rhs *PortSet) bool {
	return !equals(ps, rhs, validateOnlySharedPorts)
}

// Merge returns a new PortSet that holds the union of the two PortSets.
// Returns an error if the sets are in conflict (i.e., their intersection
// has incompatible port definitions).
func (ps *PortSet) Merge(other *PortSet) (*PortSet, error) {
	if ps.IsConflictingWith(other) {
		return nil, errors.New("Conflicting")
	}

	ports := make(map[string]mcsv1a1.ServicePort, len(ps.ports))
	for _, p := range ps.ports {
		ports[p.Name] = p
	}

	for _, p := range other.ports {
		if _, found := ports[p.Name]; !found {
			ports[p.Name] = p
		}
	}

	merged := NewPortSet()
	for _, p := range ports {
		merged.ports = append(merged.ports, p)
	}
	return merged, nil
}

// Encodes the port as an annotation. Returns the annotation name and value.
// An error is returned if either the name or value are invalid.
func encodePortAsAnnotation(port *mcsv1a1.ServicePort) (string, string, error) {
	var value string

	key := LIGHTHOUSE_PORT_KEY
	if port.Name != "" {
		key = fmt.Sprintf(LIGHTHOUSE_PORT_KEY_NAMED_FORMAT, port.Name)
	}

	if port.AppProtocol != nil && *port.AppProtocol != "" {
		value = fmt.Sprintf(LIGHTHOUSE_PORT_VALUE_FORMAT_FULL, port.Port, port.Protocol, *port.AppProtocol)
	} else if port.Protocol != "" {
		value = fmt.Sprintf(LIGHTHOUSE_PORT_VALUE_FORMAT_PROTOCOL, port.Port, port.Protocol)
	} else {
		value = fmt.Sprintf(LIGHTHOUSE_PORT_VALUE_FORMAT_PORT, port.Port)
	}

	if err := validation.IsQualifiedName(key); len(err) > 0 {
		return "", "", errors.New(err[0])
	} else if err := validation.IsValidLabelValue(value); len(err) > 0 {
		return "", "", errors.New(err[0])
	}

	return key, value, nil
}

// Decodes an annotation to determine if it matches a port specification or not.
// Returns an error when the annotation name matches but the value can't be decoded.
// Returns both error and port as nil when the annotation name does not match our
// expected format.
func decodePortFromAnnotation(name, value string) (*mcsv1a1.ServicePort, error) {
	if strings.HasPrefix(name, LIGHTHOUSE_PORT_KEY) {
		p := &mcsv1a1.ServicePort{}

		if len(name) > len(LIGHTHOUSE_PORT_KEY) {
			_, err := fmt.Sscanf(name, LIGHTHOUSE_PORT_KEY_NAMED_FORMAT, &p.Name)
			if err != nil {
				return nil, errors.New("invalid port name:" + name)
			}
		}

		// port spec is formatted as port[-[protocol]-appPort] with [] denoting
		// optional parts.
		spec := strings.Split(value, "-")
		switch len(spec) {
		case 3:
			p.AppProtocol = &spec[2]
			fallthrough
		case 2:
			p.Protocol = corev1.Protocol(spec[1])
			fallthrough
		case 1:
			i, err := strconv.Atoi(spec[0])
			if err != nil {
				return nil, err
			}
			p.Port = int32(i)
		case 0:
			return nil, errors.New("invalid port spec:" + value)
		}
		return p, nil
	}
	return nil, nil
}

// Equals determines if the two sets have equal definitions for their
// named ports. Comparison can take into account all ports in the two
// sets or just the shared set.
// According to the v1.Service definition only one port can be unnamed
// and Ports having the same name must agree on the Port, Protocol and
// AppProtocol.
func equals(lhs, rhs *PortSet, validateAllPorts bool) bool {
	if lhs == rhs {
		return true
	} else if lhs == nil || rhs == nil {
		return false
	} else if validateAllPorts && (lhs.Len() != rhs.Len()) {
		return false
	}

	ports := make(map[string]mcsv1a1.ServicePort, len(lhs.ports))
	for _, p := range lhs.ports {
		ports[p.Name] = p
	}

	for _, p := range rhs.ports {
		other, found := ports[p.Name]

		if !found {
			if !validateAllPorts {
				continue
			}
			return false
		} else if !reflect.DeepEqual(other, p) {
			return false
		}
		delete(ports, p.Name)
	}

	if validateAllPorts && len(ports) > 0 {
		return false
	}
	return true
}
