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

//go:embed templates/server.tmpl
var serverTemplateSrc string

var serverTemplate = template.Must(template.New("server.tmpl").Parse(serverTemplateSrc))

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
	svcDesc         protoreflect.ServiceDescriptor
	entries         []bindingEntry
}

type bindingEntry struct {
	typ     *gen.Type
	binding Binding
	method  protoreflect.MethodDescriptor
	sm      mixinforproto.SourceMessage
}

// renderEntry is one method's rendered output, pending the per-service
// D-20 sort-by-procedure that makes claimed and synthetic-unclaimed
// (Task 1/T-02-29) entries interleave deterministically within one
// emitted file.
type renderEntry struct {
	procedure   string
	body        string
	manualField *ManualField
}

// Generate runs the entconnect codegen pass: read every ent schema's
// RPC-binding annotations, resolve each procedure against the committed
// FileDescriptorSet (D-03, no Go import edge to any generated Connect
// package), dispatch each binding to its registered per-verb Generator,
// and emit one gofmt-clean, header-marked Go file per proto service
// (CRUD-06) into e's output directory, plus one additional combined
// server.entconnect.go declaring a single package-level NewServer that
// wires every bound service's handler into one *entconnectruntime.Server
// (D-11/D-12: one server constructor for the whole generated wiring,
// never one per proto service — a per-file NewServer would collide the
// moment a graph binds RPCs across more than one proto service, as this
// plan's own two-service Item fixture does).
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
				sb = &serviceBuild{serviceFullName: svcFullName, connectImport: connectImport, connectPkgAlias: connectAlias, svcDesc: svcDesc}
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

	// serverEntries accumulates one entry per proto service, in the same
	// deterministic svcNames order (D-20), for the single combined
	// NewServer emitted after this loop.
	var serverEntries []serverEntry

	for i, svcFullName := range svcNames {
		sb := services[svcFullName]
		sort.Slice(sb.entries, func(i, j int) bool {
			return sb.entries[i].binding.Procedure < sb.entries[j].binding.Procedure
		})

		serviceShortName := svcFullName[lastDot(svcFullName)+1:]
		structName := lowerFirst(serviceShortName) + "Server"

		var renderEntries []renderEntry
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
			renderEntries = append(renderEntries, renderEntry{
				procedure: entry.binding.Procedure, body: impl.Body, manualField: impl.ManualField,
			})
		}

		// Every method the proto service declares that no binding claimed
		// (Task 1/T-02-29): a partly-claimed service must still satisfy its
		// FULL <Service>Handler interface, so an unclaimed method gets the
		// same Manual-shaped compile-time obligation a real
		// entconnect.Manual(...) binding would — a forced app-supplied func
		// field, never a silent auto-generated no-op. These synthetic
		// entries carry no owning ent.Type and are never schema-claimed, so
		// entc/claims.go's report still names them "unclaimed" (INT-05) —
		// this is a codegen-completeness concern only, orthogonal to the
		// claims report.
		claimedMethods := make(map[protoreflect.Name]bool, len(sb.entries))
		for _, entry := range sb.entries {
			claimedMethods[entry.method.Name()] = true
		}
		ms := sb.svcDesc.Methods()
		for mi := 0; mi < ms.Len(); mi++ {
			m := ms.Get(mi)
			if claimedMethods[m.Name()] {
				continue
			}
			procedure := fmt.Sprintf("/%s/%s", sb.svcDesc.FullName(), m.Name())
			impl, err := generators[OpManual](GenRequest{
				Graph: g, Binding: Binding{Op: OpManual, Procedure: procedure}, Method: m, ServiceStructName: structName,
			})
			if err != nil {
				genFailures = append(genFailures, failure{
					schemaName: "(unclaimed)", op: string(OpManual), sortKey: svcFullName,
					rule: "generate-unclaimed", description: err.Error(), remedy: "see the error above",
				})
				continue
			}
			renderEntries = append(renderEntries, renderEntry{
				procedure: procedure, body: impl.Body, manualField: impl.ManualField,
			})
		}
		if genErr := newGenerateError(genFailures); genErr != nil {
			return genErr
		}

		sort.Slice(renderEntries, func(i, j int) bool { return renderEntries[i].procedure < renderEntries[j].procedure })

		var methodBodies []string
		var manualFields []ManualField
		for _, re := range renderEntries {
			methodBodies = append(methodBodies, re.body)
			if re.manualField != nil {
				manualFields = append(manualFields, *re.manualField)
			}
		}

		serverEntries = append(serverEntries, serverEntry{
			ServiceName:     serviceShortName,
			StructName:      structName,
			ConnectPkgAlias: sb.connectPkgAlias,
			InstanceVar:     fmt.Sprintf("svc%d", i),
			PathVar:         fmt.Sprintf("path%d", i),
			HandlerVar:      fmt.Sprintf("handler%d", i),
			ManualParams:    manualFields,
		})

		data := struct {
			ServiceName     string
			StructName      string
			ConnectPkgAlias string
			Methods         []string
			ManualFields    []ManualField
		}{
			ServiceName:     serviceShortName,
			StructName:      structName,
			ConnectPkgAlias: sb.connectPkgAlias,
			Methods:         methodBodies,
			ManualFields:    manualFields,
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

	if len(serverEntries) > 0 {
		var buf bytes.Buffer
		if err := serverTemplate.Execute(&buf, struct{ Services []serverEntry }{Services: serverEntries}); err != nil {
			return fmt.Errorf("entconnect: render server.tmpl: %w", err)
		}
		outPath := filepath.Join(outputDir, "server.entconnect.go")
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

// serverEntry is one proto service's worth of data server.tmpl needs to
// wire that service's chain-wrapped handler into the single combined
// NewServer (D-11/D-12). InstanceVar/PathVar/HandlerVar are synthesized,
// index-derived Go identifiers (never derived from ServiceName/StructName
// directly) so they can never collide with each other or with a struct
// type name across any number of bound services.
type serverEntry struct {
	ServiceName     string
	StructName      string
	ConnectPkgAlias string
	InstanceVar     string
	PathVar         string
	HandlerVar      string
	// ManualParams is this service's app-supplied handler-func slots
	// (Task 1/T-02-29), in the same deterministic procedure order the
	// struct's own ManualFields were emitted in — server.tmpl threads each
	// into both NewServer's parameter list and the service's struct
	// literal, by FieldName/ParamName/FuncType.
	ManualParams []ManualField
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
