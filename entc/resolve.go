package entc

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

// SplitProcedure splits a Connect procedure string
// ("/pkg.Service/Method" or "pkg.Service/Method") into its service full
// name and method name.
func SplitProcedure(procedure string) (service, method string, err error) {
	trimmed := strings.TrimPrefix(procedure, "/")
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		return "", "", fmt.Errorf("entconnect: malformed procedure %q: no %q separator", procedure, "/")
	}
	return trimmed[:idx], trimmed[idx+1:], nil
}

// LoadDescriptorSet reads a committed FileDescriptorSet from path and
// builds a *protoregistry.Files over it (D-03/D-04). This is the sole
// mechanism by which the entc extension resolves a procedure string to a
// descriptor — no Go import edge to any generated Connect/proto package
// is ever taken (D-02).
func LoadDescriptorSet(path string) (*protoregistry.Files, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("entconnect: read descriptor set %q: %w", path, err)
	}
	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(b, &fds); err != nil {
		return nil, fmt.Errorf("entconnect: unmarshal descriptor set %q: %w", path, err)
	}
	files, err := protodesc.NewFiles(&fds)
	if err != nil {
		return nil, fmt.Errorf("entconnect: build descriptor set %q: %w", path, err)
	}
	return files, nil
}

// ResolveMethod resolves procedure against files, the D-03 mechanism:
// split the procedure on its final "/" into service full name + method
// name, resolve the service by full name against the descriptor set,
// then look up the method by short name on the resulting
// protoreflect.ServiceDescriptor. descriptorSetPath is used only to
// render a self-sufficient D-04 error naming where to look and how to
// fix it — it is never read here.
func ResolveMethod(files *protoregistry.Files, procedure, descriptorSetPath string) (protoreflect.MethodDescriptor, error) {
	svcName, methodName, err := SplitProcedure(procedure)
	if err != nil {
		return nil, err
	}
	d, err := files.FindDescriptorByName(protoreflect.FullName(svcName))
	if err != nil {
		return nil, fmt.Errorf(
			"entconnect: procedure %q not found in %q — descriptor set may be stale; run scripts/pipeline.sh (service %q: %w)",
			procedure, descriptorSetPath, svcName, err,
		)
	}
	svcDesc, ok := d.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf(
			"entconnect: procedure %q not found in %q — descriptor set may be stale; run scripts/pipeline.sh (%q resolves to a %T, not a service)",
			procedure, descriptorSetPath, svcName, d,
		)
	}
	m := svcDesc.Methods().ByName(protoreflect.Name(methodName))
	if m == nil {
		return nil, fmt.Errorf(
			"entconnect: procedure %q not found in %q — descriptor set may be stale; run scripts/pipeline.sh (service %q has no method %q)",
			procedure, descriptorSetPath, svcName, methodName,
		)
	}
	return m, nil
}

// AllProcedures enumerates every RPC in files as "/<service.FullName>/<method.Name>"
// strings, sorted (D-07/D-20: deterministic, never map/range order). This
// is the enumeration mechanism D-07's claims report and Phase 5's drift
// check both build on.
func AllProcedures(files *protoregistry.Files) []string {
	var out []string
	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		svcs := fd.Services()
		for i := 0; i < svcs.Len(); i++ {
			s := svcs.Get(i)
			ms := s.Methods()
			for j := 0; j < ms.Len(); j++ {
				out = append(out, fmt.Sprintf("/%s/%s", s.FullName(), ms.Get(j).Name()))
			}
		}
		return true
	})
	sort.Strings(out)
	return out
}
