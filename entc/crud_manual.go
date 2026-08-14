package entc

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

func init() {
	RegisterGenerator(OpManual, generateManual)
}

//go:embed templates/manual.tmpl
var manualTemplateSrc string

var manualTemplate = template.Must(template.New("manual.tmpl").Parse(manualTemplateSrc))

// manualTemplateData is manual.tmpl's input: every value is pre-resolved Go
// source text or a plain identifier, matching every other generator's own
// "template does no descriptor traversal" discipline (D-19).
type manualTemplateData struct {
	StructName  string // e.g. "adminServiceServer" (method receiver type)
	MethodName  string // e.g. "ArchiveAdmin"
	FieldName   string // e.g. "archiveAdminFn" (the app-supplied func field)
	ReqPkgAlias string // e.g. "entconnecttestv1"
	ReqType     string // e.g. "ArchiveAdminRequest"
	RespType    string // e.g. "ArchiveAdminResponse"
	Procedure   string // e.g. "/entconnecttest.v1.AdminService/ArchiveAdmin"
}

// generateManual implements Generator for OpManual — INT-04/D-06. It
// renders: an app-supplied func field on the per-service struct (typed
// exactly like the generated <Service>Handler interface method it backs),
// and a method on that struct that calls the field, returning
// CodeUnimplemented naming the procedure when the field is nil
// (T-02-29 — a wiring mistake degrades to a clear error, never a crashed
// process). Unlike every CRUD generator, this one emits NO ent client call
// at all: the application supplies only the handler body — it never
// touches routing, the chain, or the ent client (D-06/D-12).
//
// entc/extension.go calls this generator for two distinct cases, both
// rendering identically from this function's point of view:
//   - a real entconnect.Manual(...) binding a schema declared.
//   - an unclaimed method on an otherwise-claimed proto service (Task 1's
//     "AdminService has two methods, only one is bound" case): the
//     generated struct must still satisfy the FULL <Service>Handler
//     interface, so an unclaimed method also gets a forced, compile-time-
//     visible app-supplied func slot rather than becoming a silent
//     auto-generated no-op. These synthetic entries carry no owning
//     ent.Type (req.Type is nil) and are NOT schema-claimed, so they never
//     appear as "manual" in the claims report (entc/claims.go) — only as
//     "unclaimed", which is exactly what INT-05 requires.
func generateManual(req GenRequest) (MethodImpl, error) {
	method := req.Method
	input := method.Input()
	output := method.Output()

	reqFile := input.ParentFile()
	reqPkg, reqPkgName, err := goImportPath(reqFile)
	if err != nil {
		return MethodImpl{}, fmt.Errorf("entconnect: manual %s: %w", req.Binding.Procedure, err)
	}

	methodName := string(method.Name())
	fieldName := lowerFirst(methodName) + "Fn"
	paramName := strings.TrimSuffix(req.ServiceStructName, "Server") + methodName

	data := manualTemplateData{
		StructName:  req.ServiceStructName,
		MethodName:  methodName,
		FieldName:   fieldName,
		ReqPkgAlias: reqPkgName,
		ReqType:     string(input.Name()),
		RespType:    string(output.Name()),
		Procedure:   req.Binding.Procedure,
	}

	var buf bytes.Buffer
	if err := manualTemplate.Execute(&buf, data); err != nil {
		return MethodImpl{}, fmt.Errorf("entconnect: manual %s: render manual.tmpl: %w", req.Binding.Procedure, err)
	}

	funcType := fmt.Sprintf(
		"func(context.Context, *connect.Request[%s.%s]) (*connect.Response[%s.%s], error)",
		reqPkgName, data.ReqType, reqPkgName, data.RespType,
	)

	return MethodImpl{
		Body:    buf.String(),
		Imports: []string{reqPkg, "context", "fmt", "connectrpc.com/connect"},
		ManualField: &ManualField{
			FieldName: fieldName,
			ParamName: paramName,
			FuncType:  funcType,
		},
	}, nil
}
