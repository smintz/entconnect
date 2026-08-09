package entc

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func init() {
	RegisterGenerator(OpDelete, generateDelete)
}

//go:embed templates/delete.tmpl
var deleteTemplateSrc string

var deleteTemplate = template.Must(template.New("delete.tmpl").Parse(deleteTemplateSrc))

// deleteTemplateData is delete.tmpl's input: every value is pre-resolved
// Go source text or a plain identifier — the template itself does no
// case-conversion or descriptor traversal (D-19).
type deleteTemplateData struct {
	StructName  string // e.g. "itemDeleteServiceServer" (method receiver type)
	MethodName  string // e.g. "DeleteItem"
	ReqPkgAlias string // e.g. "entconnecttestv1"
	ReqType     string // e.g. "DeleteItemRequest"
	RespType    string // e.g. "DeleteItemResponse"
	ClientField string // e.g. "Item" (s.client.<ClientField>)
	IDConvert   string // Go statements converting the request's raw lookup value into a local `id` variable of the ent ID's Go type
}

// generateDelete implements Generator for OpDelete — CRUD-02. It renders
// one DeleteItem-shaped method: parse the request's "id" field into the
// entity's ent ID type, delete by that identifier ONLY via the ent
// client's DeleteOneID builder — never a predicate-free/bulk Delete()
// call (T-02-08) — with Exec(ctx) carrying the request context so ent
// privacy policies and any policy filter apply, classify NotFound
// directly against the LOCAL generated ent package (which this
// generated file already imports), and fall through to
// runtime.MapError for anything else (privacy.Deny -> PermissionDenied,
// unrecognized -> Internal per D-18).
func generateDelete(req GenRequest) (MethodImpl, error) {
	method := req.Method
	input := method.Input()
	output := method.Output()

	reqFile := input.ParentFile()
	reqPkg, reqPkgName, err := goImportPath(reqFile)
	if err != nil {
		return MethodImpl{}, fmt.Errorf("entconnect: %s.%s: %w", req.Type.Name, req.Binding.Op, err)
	}

	idField := input.Fields().ByName(protoreflect.Name("id"))
	if idField == nil {
		return MethodImpl{}, fmt.Errorf(
			"entconnect: %s.%s: request message %q has no \"id\" field — DeleteRPC requires the request to carry a lookup field literally named \"id\" (add one to the proto message)",
			req.Type.Name, req.Binding.Op, input.FullName(),
		)
	}

	idGoType := "int"
	if req.Type.ID != nil && req.Type.ID.Type != nil {
		idGoType = req.Type.ID.Type.String()
	}
	idLookupExpr := "req.Msg.Get" + goCamelCase(string(idField.Name())) + "()"
	idConvert, err := idConversion(idGoType, idLookupExpr)
	if err != nil {
		return MethodImpl{}, fmt.Errorf("entconnect: %s.%s: %w", req.Type.Name, req.Binding.Op, err)
	}

	data := deleteTemplateData{
		StructName:  req.ServiceStructName,
		MethodName:  string(method.Name()),
		ReqPkgAlias: reqPkgName,
		ReqType:     string(input.Name()),
		RespType:    string(output.Name()),
		ClientField: req.Type.Name,
		IDConvert:   idConvert,
	}

	var buf bytes.Buffer
	if err := deleteTemplate.Execute(&buf, data); err != nil {
		return MethodImpl{}, fmt.Errorf("entconnect: %s.%s: render delete.tmpl: %w", req.Type.Name, req.Binding.Op, err)
	}

	return MethodImpl{
		Body: buf.String(),
		Imports: []string{
			reqPkg,
			"context",
			"connectrpc.com/connect",
		},
	}, nil
}
