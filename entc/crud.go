package entc

import (
	"fmt"

	"entgo.io/ent/entc/gen"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/smintz/entconnect/mixinforproto"
)

// GenRequest carries everything one per-verb Generator needs to render
// one generated RPC method body.
type GenRequest struct {
	// Graph is the loaded ent schema graph.
	Graph *gen.Graph
	// Type is the ent entity type the binding was declared on.
	Type *gen.Type
	// Binding is the resolved Op + procedure this method implements.
	Binding Binding
	// Method is the resolved protoreflect.MethodDescriptor for
	// Binding.Procedure.
	Method protoreflect.MethodDescriptor
	// SourceMessage is Phase 1's provenance annotation for Type — the
	// proto message Type's fields were derived from.
	SourceMessage mixinforproto.SourceMessage
	// ServiceStructName is the unexported Go identifier of the
	// per-service struct this method is a receiver method on (e.g.
	// "orderReadServiceServer"), computed once per service by
	// extension.go and threaded into every binding on that service.
	ServiceStructName string
}

// MethodImpl is one generator's rendered output: the Go source text of
// one <Service>Handler method body, plus the import paths that text
// references. Import paths are informational/documentation — the
// extension's emission hook (extension.go) runs every rendered file
// through golang.org/x/tools/imports.Process before writing, which
// resolves and inserts the actual import block from the rendered source
// text directly, so a Generator's Imports value is not itself consumed
// by the writer. It is still recorded so a future generator author (or a
// golden-file test) can see at a glance what a given verb pulls in
// without re-deriving it from the rendered text.
type MethodImpl struct {
	Body    string
	Imports []string
	// ManualField, when non-nil, is an app-supplied func field this
	// method's generator needs added to the per-service struct (Task 1,
	// INT-04/D-06): an explicit Manual binding, or an unclaimed method on
	// an otherwise-claimed service (T-02-29's compile-time obligation —
	// never a silent auto-stub). Only entc/crud_manual.go's generator
	// populates this.
	ManualField *ManualField
}

// ManualField is one app-supplied handler-func slot a generated
// per-service struct exposes: a struct field (FieldName) of type
// FuncType, and the corresponding NewServer constructor parameter name
// (ParamName) the application passes its handler func through. FuncType
// is shared verbatim between the struct field declaration
// (entc/templates/service.tmpl) and the NewServer parameter declaration
// (entc/templates/server.tmpl) so the two can never drift apart.
type ManualField struct {
	FieldName string
	ParamName string
	FuncType  string
}

// Generator renders one CRUD verb's method body from req.
type Generator func(GenRequest) (MethodImpl, error)

// generators is the per-Op registry later plans (Create/Update/Delete/List)
// populate via RegisterGenerator's init()-time calls, so adding a verb
// never requires editing this file.
var generators = map[Op]Generator{}

// RegisterGenerator registers g as the Generator for op. Panics on a
// duplicate registration for the same Op — a programming error in this
// package, not a runtime/user condition.
func RegisterGenerator(op Op, g Generator) {
	if _, exists := generators[op]; exists {
		panic(fmt.Sprintf("entconnect: duplicate generator registration for op %q", op))
	}
	generators[op] = g
}

// goFileOptions returns fd's *descriptorpb.FileOptions, or nil if fd
// carries none.
func goFileOptions(fd protoreflect.FileDescriptor) *descriptorpb.FileOptions {
	opts, _ := fd.Options().(*descriptorpb.FileOptions)
	return opts
}

// goImportPath returns the Go import path and local package identifier
// fd's go_package file option names. This is read directly off the
// committed FileDescriptorSet (fd.Options()) — never derived from a Go
// import of the generated package, keeping D-02's "no import edge to any
// generated Connect/proto package" guarantee intact even for this
// code-emission concern. go_package must therefore be set explicitly in
// proto source for any file entc/ needs to reference from generated
// code (buf.gen.yaml's managed-mode override alone is generate-time only
// and never reaches the descriptor set — see read.proto's own comment).
func goImportPath(fd protoreflect.FileDescriptor) (importPath, pkgName string, err error) {
	opts := goFileOptions(fd)
	gp := opts.GetGoPackage()
	if gp == "" {
		return "", "", fmt.Errorf(
			"entconnect: file %q has no go_package option — add `option go_package = \"...\";` to its proto source and re-run scripts/pipeline.sh",
			fd.Path(),
		)
	}
	if idx := lastIndexByte(gp, ';'); idx >= 0 {
		return gp[:idx], gp[idx+1:], nil
	}
	name := gp
	for i := len(gp) - 1; i >= 0; i-- {
		if gp[i] == '/' {
			name = gp[i+1:]
			break
		}
	}
	return gp, name, nil
}

func lastIndexByte(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// connectImportPath derives the protoc-gen-connect-go sibling package
// (<pkg>connect, in a <pkg>connect/ subdirectory next to the message
// package — protoc-gen-connect-go's own, well-documented convention) for
// the file svcFD belongs to.
func connectImportPath(svcFD protoreflect.FileDescriptor) (importPath, pkgName string, err error) {
	base, name, err := goImportPath(svcFD)
	if err != nil {
		return "", "", err
	}
	return base + "connect", name + "connect", nil
}

// goCamelCase camel-cases a protobuf field name for use as a Go struct
// field identifier, matching protoc-gen-go's own naming algorithm
// exactly (google.golang.org/protobuf/internal/strs.GoCamelCase — copied
// here since that package is internal/ and not importable). "id" ->
// "Id", "created_at" -> "CreatedAt": no acronym special-casing.
func goCamelCase(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_' && (i == 0):
			b = append(b, 'X')
		case c == '_' && i+1 < len(s) && isASCIILower(s[i+1]):
			// drop the underscore; next loop iteration upper-cases the
			// following letter via the default branch below
		case isASCIIDigit(c):
			b = append(b, c)
		default:
			if isASCIILower(c) {
				c -= 'a' - 'A'
			}
			b = append(b, c)
			for ; i+1 < len(s) && isASCIILower(s[i+1]); i++ {
				b = append(b, s[i+1])
			}
		}
	}
	return string(b)
}

func isASCIILower(c byte) bool { return 'a' <= c && c <= 'z' }
func isASCIIDigit(c byte) bool { return '0' <= c && c <= '9' }
