package entc

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/template"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"golang.org/x/tools/imports"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/smintz/entconnect/mixinforproto"
)

//go:embed templates/service.tmpl
var serviceTemplateSrc string

var serviceTemplate = template.Must(template.New("service.tmpl").Parse(serviceTemplateSrc))

// DefaultDescriptorSetPath is the descriptor-set path WithDescriptorSet
// defaults to when not overridden — the pipeline's committed path
// (scripts/pipeline.sh's DESCRIPTOR_OUT).
const DefaultDescriptorSetPath = "proto/descriptorset.binpb"

// Extension is the entconnect entc.Extension (D-19), mirroring
// entgo.io/contrib/entproto's own Extension/NewExtension/Hooks()
// skeleton.
type Extension struct {
	entc.DefaultExtension
	descriptorSetPath string
	outputDir         string
}

// ExtensionOption configures an Extension.
type ExtensionOption func(*Extension) error

// WithDescriptorSet overrides the committed FileDescriptorSet path
// (D-04). Defaults to DefaultDescriptorSetPath.
func WithDescriptorSet(path string) ExtensionOption {
	return func(e *Extension) error {
		e.descriptorSetPath = path
		return nil
	}
}

// WithOutputDir overrides the directory generated handler files are
// written to. Defaults to a sibling of the app's ent/ package —
// filepath.Join(filepath.Dir(g.Config.Target), "entconnect") — computed
// at generate time, never inside ent/ itself (CRUD-07: generated wiring
// lives outside the app's public ent package).
func WithOutputDir(dir string) ExtensionOption {
	return func(e *Extension) error {
		e.outputDir = dir
		return nil
	}
}

// NewExtension builds an Extension.
func NewExtension(opts ...ExtensionOption) (*Extension, error) {
	e := &Extension{descriptorSetPath: DefaultDescriptorSetPath}
	for _, opt := range opts {
		if err := opt(e); err != nil {
			return nil, err
		}
	}
	return e, nil
}

// Hooks implements entc.Extension.
func (e *Extension) Hooks() []gen.Hook {
	return []gen.Hook{e.hook()}
}

func (e *Extension) hook() gen.Hook {
	return func(next gen.Generator) gen.Generator {
		return gen.GenerateFunc(func(g *gen.Graph) error {
			if err := next.Generate(g); err != nil {
				return err
			}
			return Generate(g, e)
		})
	}
}

// serviceBuild accumulates one proto service's worth of resolved
// bindings while Generate walks the schema graph, before it is rendered
// into one output file.
type serviceBuild struct {
	serviceFullName string
	connectImport   string
	connectPkgAlias string
	entries         []bindingEntry
}

type bindingEntry struct {
	typ     *gen.Type
	binding Binding
	method  protoreflect.MethodDescriptor
	sm      mixinforproto.SourceMessage
}

// Generate runs the entconnect codegen pass: read every ent schema's
// RPC-binding annotations, resolve each procedure against the committed
// FileDescriptorSet (D-03, no Go import edge to any generated Connect
// package), dispatch each binding to its registered per-verb Generator,
// and emit one gofmt-clean, header-marked Go file per proto service
// (CRUD-06) into e's output directory.
func Generate(g *gen.Graph, e *Extension) error {
	outputDir := e.outputDir
	if outputDir == "" {
		outputDir = filepath.Join(filepath.Dir(g.Config.Target), "entconnect")
	}

	files, err := LoadDescriptorSet(e.descriptorSetPath)
	if err != nil {
		return err
	}

	nodes := make([]*gen.Type, len(g.Nodes))
	copy(nodes, g.Nodes)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })

	var failures []failure
	services := map[string]*serviceBuild{}
	claimed := map[string]string{} // procedure -> "schema.op"

	for _, typ := range nodes {
		bindings, err := decodeAnnotation[Bindings](typ.Annotations, RPCBindings)
		if err != nil {
			// No RPC-binding annotation on this schema at all — not an
			// error, just an entity this codegen pass has nothing to do
			// for (D-07's "unclaimed" reporting is a later plan's
			// concern, not a build failure here).
			continue
		}
		for _, b := range bindings.Bindings {
			method, err := ResolveMethod(files, b.Procedure, e.descriptorSetPath)
			if err != nil {
				failures = append(failures, failure{
					schemaName: typ.Name, op: string(b.Op), sortKey: typ.Name,
					rule: "resolve", description: err.Error(),
					remedy: "descriptor set may be stale; run scripts/pipeline.sh",
				})
				continue
			}
			if prevClaimant, ok := claimed[b.Procedure]; ok {
				failures = append(failures, failure{
					schemaName: typ.Name, op: string(b.Op), sortKey: typ.Name,
					rule: "duplicate-claim",
					description: fmt.Sprintf(
						"procedure %q is claimed by both %s and %s.%s",
						b.Procedure, prevClaimant, typ.Name, b.Op,
					),
					remedy: "remove the duplicate binding — a procedure may be claimed by exactly one schema/op (D-05)",
				})
				continue
			}
			claimed[b.Procedure] = fmt.Sprintf("%s.%s", typ.Name, b.Op)

			sm, err := decodeAnnotation[mixinforproto.SourceMessage](typ.Annotations, mixinforproto.MixinForProtoMessage)
			if err != nil {
				failures = append(failures, failure{
					schemaName: typ.Name, op: string(b.Op), sortKey: typ.Name,
					rule: "decode-source-message",
					description: fmt.Sprintf(
						"schema %q binds procedure %q but has no MixinForProto provenance annotation (%v)",
						typ.Name, b.Procedure, err,
					),
					remedy: "GetRPC/CreateRPC/... require the schema's Mixin() to include mixinforproto.MixinForProto[...]",
				})
				continue
			}

			svcDesc, ok := method.Parent().(protoreflect.ServiceDescriptor)
			if !ok {
				failures = append(failures, failure{
					schemaName: typ.Name, op: string(b.Op), sortKey: typ.Name,
					rule: "resolve", description: fmt.Sprintf("procedure %q does not resolve to a service method", b.Procedure),
					remedy: "descriptor set may be stale; run scripts/pipeline.sh",
				})
				continue
			}
			svcFullName := string(svcDesc.FullName())
			sb, ok := services[svcFullName]
			if !ok {
				connectImport, connectAlias, err := connectImportPath(svcDesc.ParentFile())
				if err != nil {
					failures = append(failures, failure{
						schemaName: typ.Name, op: string(b.Op), sortKey: typ.Name,
						rule: "go-import-path", description: err.Error(),
						remedy: "add `option go_package = \"...\";` to the proto file declaring this service",
					})
					continue
				}
				sb = &serviceBuild{serviceFullName: svcFullName, connectImport: connectImport, connectPkgAlias: connectAlias}
				services[svcFullName] = sb
			}
			sb.entries = append(sb.entries, bindingEntry{typ: typ, binding: b, method: method, sm: sm})
		}
	}

	if genErr := newGenerateError(failures); genErr != nil {
		return genErr
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("entconnect: create output dir %q: %w", outputDir, err)
	}

	svcNames := make([]string, 0, len(services))
	for name := range services {
		svcNames = append(svcNames, name)
	}
	sort.Strings(svcNames)

	for _, svcFullName := range svcNames {
		sb := services[svcFullName]
		sort.Slice(sb.entries, func(i, j int) bool {
			return sb.entries[i].binding.Procedure < sb.entries[j].binding.Procedure
		})

		serviceShortName := svcFullName[lastDot(svcFullName)+1:]
		structName := lowerFirst(serviceShortName) + "Server"

		var methodBodies []string
		var genFailures []failure
		for _, entry := range sb.entries {
			gen, ok := generators[entry.binding.Op]
			if !ok {
				genFailures = append(genFailures, failure{
					schemaName: entry.typ.Name, op: string(entry.binding.Op), sortKey: entry.typ.Name,
					rule:        "unregistered-op",
					description: fmt.Sprintf("op %q has no registered generator", entry.binding.Op),
					remedy:      "this Op is not implemented by this version of entconnect's entc extension yet",
				})
				continue
			}
			impl, err := gen(GenRequest{
				Graph: g, Type: entry.typ, Binding: entry.binding, Method: entry.method,
				SourceMessage: entry.sm, ServiceStructName: structName,
			})
			if err != nil {
				genFailures = append(genFailures, failure{
					schemaName: entry.typ.Name, op: string(entry.binding.Op), sortKey: entry.typ.Name,
					rule: "generate", description: err.Error(), remedy: "see the error above",
				})
				continue
			}
			methodBodies = append(methodBodies, impl.Body)
		}
		if genErr := newGenerateError(genFailures); genErr != nil {
			return genErr
		}

		data := struct {
			ServiceName     string
			StructName      string
			ConnectPkgAlias string
			Methods         []string
		}{
			ServiceName:     serviceShortName,
			StructName:      structName,
			ConnectPkgAlias: sb.connectPkgAlias,
			Methods:         methodBodies,
		}

		var buf bytes.Buffer
		if err := serviceTemplate.Execute(&buf, data); err != nil {
			return fmt.Errorf("entconnect: render service.tmpl for %q: %w", svcFullName, err)
		}

		outPath := filepath.Join(outputDir, snakeCase(serviceShortName)+".entconnect.go")
		formatted, err := imports.Process(outPath, buf.Bytes(), nil)
		if err != nil {
			return fmt.Errorf("entconnect: gofmt/goimports %q: %w", outPath, err)
		}
		if err := os.WriteFile(outPath, formatted, 0o644); err != nil {
			return fmt.Errorf("entconnect: write %q: %w", outPath, err)
		}
	}

	return nil
}

func lastDot(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'A' && b[0] <= 'Z' {
		b[0] += 'a' - 'A'
	}
	return string(b)
}

// snakeCase converts a PascalCase identifier (e.g. "OrderReadService")
// into snake_case ("order_read_service") for the emitted filename.
func snakeCase(s string) string {
	var b []byte
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b = append(b, '_')
			}
			b = append(b, byte(r-'A'+'a'))
			continue
		}
		b = append(b, byte(r))
	}
	return string(b)
}
