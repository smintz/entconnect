package entc

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func init() {
	RegisterGenerator(OpGet, generateGet)
}

//go:embed templates/get.tmpl
var getTemplateSrc string

var getTemplate = template.Must(template.New("get.tmpl").Parse(getTemplateSrc))

// getTemplateData is get.tmpl's input: every value is pre-resolved Go
// source text or a plain identifier — the template itself does no
// case-conversion or descriptor traversal, keeping it a plain
// text/template (D-19) rather than needing custom template functions.
type getTemplateData struct {
	StructName      string // e.g. "orderReadServiceServer" (method receiver type)
	MethodName      string // e.g. "GetOrder"
	ReqPkgAlias     string // e.g. "entconnecttestv1"
	ReqType         string // e.g. "GetOrderRequest"
	RespType        string // e.g. "GetOrderResponse"
	EntityType      string // e.g. "Order" (the ReqPkgAlias.EntityType Go type)
	RespEntityField string // e.g. "Order" (the Go field on RespType holding the entity)
	ClientField     string // e.g. "Order" (s.client.<ClientField>)
	IDLookupExpr    string // Go expression yielding the request's raw lookup value, e.g. "req.Msg.GetId()"
	IDConvert       string // Go statements converting IDLookupExpr into a local `id` variable of the ent ID's Go type, or a build-time-validated InvalidArgument return
	IDGoType        string // ent's ID Go type, e.g. "int"
	SetFields       []getSetField
}

type getSetField struct {
	// GoName is the response entity message's Go field name (e.g. "Customer").
	GoName string
	// ValueExpr is the Go expression reading the corresponding value off
	// the fetched ent row (e.g. "row.Customer" or "timestamppb.New(row.CreatedAt)").
	ValueExpr string
}

// generateGet implements Generator for OpGet — CRUD-01. It renders one
// GetOrder-shaped method: parse the request's "id" field into the
// entity's ent ID type, fetch via client.<Type>.Get(ctx, id) (with ctx
// carrying the viewer the chain injected, so privacy policies apply),
// classify NotFound/ConstraintError/ValidationError against the LOCAL
// generated ent package (which this generated file already imports),
// fall through to runtime.MapError for anything else, and build the
// response by converting every derived ent field back into its proto
// counterpart.
func generateGet(req GenRequest) (MethodImpl, error) {
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
			"entconnect: %s.%s: request message %q has no \"id\" field — GetRPC requires the request to carry a lookup field literally named \"id\" (add one to the proto message)",
			req.Type.Name, req.Binding.Op, input.FullName(),
		)
	}

	entityFullName := protoreflect.FullName(req.SourceMessage.Message)
	var respEntityField protoreflect.FieldDescriptor
	outFields := output.Fields()
	for i := 0; i < outFields.Len(); i++ {
		fd := outFields.Get(i)
		if fd.Kind() == protoreflect.MessageKind && fd.Message().FullName() == entityFullName {
			respEntityField = fd
			break
		}
	}
	if respEntityField == nil {
		return MethodImpl{}, fmt.Errorf(
			"entconnect: %s.%s: response message %q has no field of type %q to hold the resolved entity",
			req.Type.Name, req.Binding.Op, output.FullName(), entityFullName,
		)
	}

	entityFullNameStr := string(entityFullName)
	entityShortName := entityFullNameStr[strings.LastIndex(entityFullNameStr, ".")+1:]

	idGoType := "int"
	if req.Type.ID != nil && req.Type.ID.Type != nil {
		idGoType = req.Type.ID.Type.String()
	}
	idLookupExpr := "req.Msg.Get" + goCamelCase(string(idField.Name())) + "()"
	idConvert, err := idConversion(idGoType, idLookupExpr)
	if err != nil {
		return MethodImpl{}, fmt.Errorf("entconnect: %s.%s: %w", req.Type.Name, req.Binding.Op, err)
	}

	setFields := make([]getSetField, 0, len(req.Type.Fields))
	for _, f := range req.Type.Fields {
		goName := goCamelCase(f.Name)
		valueExpr := "row." + goCamelCase(f.Name)
		if f.Type != nil && f.Type.Type == field.TypeTime {
			valueExpr = "timestamppb.New(" + valueExpr + ")"
		}
		setFields = append(setFields, getSetField{GoName: goName, ValueExpr: valueExpr})
	}

	data := getTemplateData{
		StructName:      req.ServiceStructName,
		MethodName:      string(method.Name()),
		ReqPkgAlias:     reqPkgName,
		ReqType:         string(input.Name()),
		RespType:        string(output.Name()),
		EntityType:      entityShortName,
		RespEntityField: goCamelCase(string(respEntityField.Name())),
		ClientField:     req.Type.Name,
		IDLookupExpr:    idLookupExpr,
		IDConvert:       idConvert,
		IDGoType:        idGoType,
		SetFields:       setFields,
	}

	var buf bytes.Buffer
	if err := getTemplate.Execute(&buf, data); err != nil {
		return MethodImpl{}, fmt.Errorf("entconnect: %s.%s: render get.tmpl: %w", req.Type.Name, req.Binding.Op, err)
	}

	return MethodImpl{
		Body: buf.String(),
		Imports: []string{
			reqPkg,
			"context",
			"fmt",
			"connectrpc.com/connect",
			"google.golang.org/protobuf/types/known/timestamppb",
		},
	}, nil
}

// idConversion returns the Go statements that parse lookupExpr (the
// request's raw string lookup value) into a local `id` variable of
// goType, or a well-formed generateGet error naming an unsupported ID
// kind — never a silent guess.
func idConversion(goType, lookupExpr string) (string, error) {
	switch goType {
	case "string":
		return "id := " + lookupExpr, nil
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return fmt.Sprintf(
			"parsedID, err := strconv.ParseInt(%s, 10, 64)\n\tif err != nil {\n\t\treturn nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf(\"invalid id: %%w\", err))\n\t}\n\tid := %s(parsedID)",
			lookupExpr, goType,
		), nil
	default:
		return "", fmt.Errorf("unsupported ent ID Go type %q for GetRPC — only string and integer ID types are supported", goType)
	}
}
