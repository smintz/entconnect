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
	RegisterGenerator(OpCreate, generateCreate)
}

//go:embed templates/create.tmpl
var createTemplateSrc string

var createTemplate = template.Must(template.New("create.tmpl").Parse(createTemplateSrc))

// createTemplateData is create.tmpl's input: every value is pre-resolved
// Go source text or a plain identifier — the template itself does no
// case-conversion or descriptor traversal (D-19).
type createTemplateData struct {
	StructName      string // e.g. "itemCreateServiceServer" (method receiver type)
	MethodName      string // e.g. "CreateItem"
	ReqPkgAlias     string // e.g. "entconnecttestv1"
	ReqType         string // e.g. "CreateItemRequest"
	RespType        string // e.g. "CreateItemResponse"
	EntityType      string // e.g. "Item" (the ReqPkgAlias.EntityType Go type)
	RespEntityField string // e.g. "Item" (the Go field on RespType holding the entity)
	ClientField     string // e.g. "Item" (s.client.<ClientField>)
	SetCalls        []createSetCall
	RespSetFields   []createRespField
}

type createSetCall struct {
	// SetterName is the ent create-builder method, e.g. "SetSku".
	SetterName string
	// ValueExpr is the Go expression reading the corresponding value off
	// the request's entity message (e.g. "req.Msg.GetItem().GetSku()").
	ValueExpr string
}

type createRespField struct {
	// GoName is the response entity message's Go field name (e.g. "Sku").
	GoName string
	// ValueExpr is the Go expression reading the corresponding value off
	// the persisted ent row (e.g. "row.Sku").
	ValueExpr string
}

// generateCreate implements Generator for OpCreate — CRUD-02. It renders
// one CreateItem-shaped method: build the ent create builder with one
// Set<Field> call per surviving contract field (skipping any field the
// mixin excluded and the ent-generated primary key, determined via
// req.Type.ID rather than name matching), call Save(ctx) with the
// request context so ent privacy policies see the injected viewer,
// route ConstraintError/ValidationError directly against the LOCAL
// generated ent package (which this generated file already imports),
// fall through to runtime.MapError for anything else (privacy.Deny ->
// PermissionDenied, unrecognized -> Internal per D-18), and build the
// response by echoing every derived ent field back from the persisted
// row.
func generateCreate(req GenRequest) (MethodImpl, error) {
	method := req.Method
	input := method.Input()
	output := method.Output()

	reqFile := input.ParentFile()
	reqPkg, reqPkgName, err := goImportPath(reqFile)
	if err != nil {
		return MethodImpl{}, fmt.Errorf("entconnect: %s.%s: %w", req.Type.Name, req.Binding.Op, err)
	}

	entityFullName := protoreflect.FullName(req.SourceMessage.Message)

	reqEntityField, err := findEntityField(input.Fields(), entityFullName)
	if err != nil {
		return MethodImpl{}, fmt.Errorf(
			"entconnect: %s.%s: request message %q has no field of type %q to carry the entity to persist",
			req.Type.Name, req.Binding.Op, input.FullName(), entityFullName,
		)
	}
	respEntityField, err := findEntityField(output.Fields(), entityFullName)
	if err != nil {
		return MethodImpl{}, fmt.Errorf(
			"entconnect: %s.%s: response message %q has no field of type %q to hold the persisted entity",
			req.Type.Name, req.Binding.Op, output.FullName(), entityFullName,
		)
	}

	entityFullNameStr := string(entityFullName)
	entityShortName := entityFullNameStr[strings.LastIndex(entityFullNameStr, ".")+1:]
	reqEntityGoName := goCamelCase(string(reqEntityField.Name()))

	excluded := make(map[string]bool, len(req.SourceMessage.Excluded))
	for _, name := range req.SourceMessage.Excluded {
		excluded[name] = true
	}
	fieldsByName := make(map[string]*fieldRef, len(req.Type.Fields))
	for _, f := range req.Type.Fields {
		goName := goCamelCase(f.Name)
		valueExpr := "row." + goName
		if f.Type != nil && f.Type.Type == field.TypeTime {
			valueExpr = "timestamppb.New(" + valueExpr + ")"
		}
		fieldsByName[f.Name] = &fieldRef{goName: goName, respValueExpr: valueExpr}
	}

	idName := ""
	if req.Type.ID != nil {
		idName = req.Type.ID.Name
	}

	// SourceMessage.Fields is the complete descriptor field inventory in
	// descriptor declaration order (D-20: never map/range order) — the
	// canonical iteration order for every surviving Set<Field> call.
	var setCalls []createSetCall
	var respSetFields []createRespField
	for _, fr := range req.SourceMessage.Fields {
		if excluded[fr.Name] || fr.Name == idName {
			continue
		}
		f, ok := fieldsByName[fr.Name]
		if !ok {
			// Overridden or otherwise unmapped field: nothing to Set.
			continue
		}
		setCalls = append(setCalls, createSetCall{
			SetterName: "Set" + f.goName,
			ValueExpr:  "req.Msg.Get" + reqEntityGoName + "().Get" + f.goName + "()",
		})
		respSetFields = append(respSetFields, createRespField{
			GoName:    f.goName,
			ValueExpr: f.respValueExpr,
		})
	}

	data := createTemplateData{
		StructName:      req.ServiceStructName,
		MethodName:      string(method.Name()),
		ReqPkgAlias:     reqPkgName,
		ReqType:         string(input.Name()),
		RespType:        string(output.Name()),
		EntityType:      entityShortName,
		RespEntityField: goCamelCase(string(respEntityField.Name())),
		ClientField:     req.Type.Name,
		SetCalls:        setCalls,
		RespSetFields:   respSetFields,
	}

	var buf bytes.Buffer
	if err := createTemplate.Execute(&buf, data); err != nil {
		return MethodImpl{}, fmt.Errorf("entconnect: %s.%s: render create.tmpl: %w", req.Type.Name, req.Binding.Op, err)
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

// fieldRef is a small lookup value for one ent field, computed once and
// reused for both the Set<Field> call and the echoed response field.
type fieldRef struct {
	goName        string
	respValueExpr string
}

// findEntityField returns the field in fields whose type is a message of
// entityFullName, or an error if none exists. Used to locate the request
// message's "carry the entity" field (Create) and the response message's
// "hold the resolved/persisted entity" field (Get/Create), mirroring
// generateGet's identical inline search.
func findEntityField(fields protoreflect.FieldDescriptors, entityFullName protoreflect.FullName) (protoreflect.FieldDescriptor, error) {
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.Kind() == protoreflect.MessageKind && fd.Message().FullName() == entityFullName {
			return fd, nil
		}
	}
	return nil, fmt.Errorf("no field of type %q", entityFullName)
}
