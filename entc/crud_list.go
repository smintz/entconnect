package entc

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/smintz/entconnect/mixinforproto"
)

func init() {
	RegisterGenerator(OpList, generateList)
}

//go:embed templates/list.tmpl
var listTemplateSrc string

var listTemplate = template.Must(template.New("list.tmpl").Parse(listTemplateSrc))

// listPageSizeDefault and listPageSizeMax are T-02-13's clamp bounds,
// the single source of truth list.tmpl's rendered named constants take
// their values from: page_size is clamped into [1, listPageSizeMax]
// with listPageSizeDefault applied when the request's page_size is zero
// or negative, so no generated query can ever be unbounded.
const (
	listPageSizeDefault = 50
	listPageSizeMax     = 100
)

// listFieldSpec is one column of the keyset ordering tuple: a business
// ordering field (from Binding.OrderBy) or, always last, the entity's
// primary key. GTFunc/EQFunc/OrderFunc are bare (unprefixed) identifiers
// on the entity's ent predicate/order package — list.tmpl prefixes them
// with EntityPkg at render time.
type listFieldSpec struct {
	GoName     string // e.g. "CreatedAt" or "ID"
	GoType     string // the ent field's Go type string, e.g. "time.Time", "int" — drives DecodeStmt/EncodeExpr's shape
	ValueExpr  string // e.g. "row.CreatedAt" or "row.ID" — reading the field off a fetched row
	VarName    string // e.g. "keyCreatedAt" — the local variable DecodeStmt assigns
	GTFunc     string // e.g. "CreatedAtGT"
	EQFunc     string // e.g. "CreatedAtEQ"
	OrderFunc  string // e.g. "ByCreatedAt"
	EncodeExpr string // Go expression converting the LAST row's value into a cursor key string, e.g. "last.CreatedAt.Format(time.RFC3339Nano)"
	DecodeStmt string // Go statement(s) assigning VarName from cursor.Keys[i], or returning a well-formed InvalidArgument error on parse failure
}

// listGoKind classifies the Go types list ordering fields may use — the
// only kinds this generator knows how to round-trip through a string
// cursor key.
type listGoKind int

const (
	listKindString listGoKind = iota
	listKindInt
	listKindTime
)

func classifyListGoType(goType string) (listGoKind, error) {
	switch goType {
	case "string":
		return listKindString, nil
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return listKindInt, nil
	case "time.Time":
		return listKindTime, nil
	default:
		return 0, fmt.Errorf(
			"unsupported ordering field Go type %q — ListRPC ordering supports string, integer, and time.Time ent fields only",
			goType,
		)
	}
}

// listTemplateData is list.tmpl's input: like getTemplateData, every
// value is pre-resolved Go source text or a plain identifier (D-19 — a
// plain text/template, no descriptor traversal inside the template
// itself).
type listTemplateData struct {
	StructName     string
	MethodName     string
	ReqPkgAlias    string
	ReqType        string
	RespType       string
	EntityType     string
	RespListField  string // e.g. "Pages"
	NextTokenField string // e.g. "NextPageToken"
	ClientField    string // e.g. "Page" (s.client.<ClientField>)
	EntityPkg      string // e.g. "page" — the ent predicate/order package's local identifier
	Fields         []listFieldSpec
	FieldCount     int
	WhereExpr      string // the full Or(...)/And(...) keyset predicate expression, EntityPkg-prefixed
	SetFields      []getSetField
	DefaultConst   string // e.g. "ListPagesDefaultPageSize"
	DefaultValue   int    // listPageSizeDefault
	MaxConst       string // e.g. "ListPagesMaxPageSize"
	MaxValue       int    // listPageSizeMax
}

// generateList implements Generator for OpList — CRUD-03. It renders a
// List method that pages entirely via keyset predicates over ent's own
// per-field comparison operators (RESEARCH.md Pattern 2): a Limit of
// pageSize+1 for next-page detection, an Order call with one OrderOption
// per ordering-tuple column (all ascending), and — when page_token is
// present — a decoded, fingerprint-checked cursor spliced into a Where
// clause built as the standard lexicographic keyset disjunction. Never
// renders a skip/limit-style pagination call.
func generateList(req GenRequest) (MethodImpl, error) {
	method := req.Method
	input := method.Input()
	output := method.Output()

	reqFile := input.ParentFile()
	reqPkg, reqPkgName, err := goImportPath(reqFile)
	if err != nil {
		return MethodImpl{}, fmt.Errorf("entconnect: %s.%s: %w", req.Type.Name, req.Binding.Op, err)
	}

	pageSizeField := input.Fields().ByName(protoreflect.Name("page_size"))
	if pageSizeField == nil {
		return MethodImpl{}, fmt.Errorf(
			"entconnect: %s.%s: request message %q has no \"page_size\" field — ListRPC requires an AIP-158-shaped request (page_size, page_token)",
			req.Type.Name, req.Binding.Op, input.FullName(),
		)
	}
	pageTokenField := input.Fields().ByName(protoreflect.Name("page_token"))
	if pageTokenField == nil {
		return MethodImpl{}, fmt.Errorf(
			"entconnect: %s.%s: request message %q has no \"page_token\" field — ListRPC requires an AIP-158-shaped request (page_size, page_token)",
			req.Type.Name, req.Binding.Op, input.FullName(),
		)
	}

	nextTokenField := output.Fields().ByName(protoreflect.Name("next_page_token"))
	if nextTokenField == nil {
		return MethodImpl{}, fmt.Errorf(
			"entconnect: %s.%s: response message %q has no \"next_page_token\" field — ListRPC requires an AIP-158-shaped response",
			req.Type.Name, req.Binding.Op, output.FullName(),
		)
	}

	entityFullName := protoreflect.FullName(req.SourceMessage.Message)
	var respListField protoreflect.FieldDescriptor
	outFields := output.Fields()
	for i := 0; i < outFields.Len(); i++ {
		fd := outFields.Get(i)
		if fd.Cardinality() == protoreflect.Repeated && fd.Kind() == protoreflect.MessageKind && fd.Message().FullName() == entityFullName {
			respListField = fd
			break
		}
	}
	if respListField == nil {
		return MethodImpl{}, fmt.Errorf(
			"entconnect: %s.%s: response message %q has no repeated field of type %q to hold the page's rows",
			req.Type.Name, req.Binding.Op, output.FullName(), entityFullName,
		)
	}

	entityFullNameStr := string(entityFullName)
	entityShortName := entityFullNameStr[strings.LastIndex(entityFullNameStr, ".")+1:]

	fields, err := listOrderFields(req)
	if err != nil {
		return MethodImpl{}, err
	}
	// DecodeStmt is index-dependent (cursor.Keys[i]) — filled in here, once
	// the full, final tuple order is known.
	for i := range fields {
		fields[i].DecodeStmt = decodeStmtFor(fields[i], i)
	}

	entityPkg := req.Type.Package()

	setFields := make([]getSetField, 0, len(req.Type.Fields))
	for _, f := range req.Type.Fields {
		goName := goCamelCase(f.Name)
		valueExpr := "row." + goName
		if f.Type != nil && f.Type.Type == field.TypeTime {
			valueExpr = "timestamppb.New(" + valueExpr + ")"
		}
		setFields = append(setFields, getSetField{GoName: goName, ValueExpr: valueExpr})
	}

	whereExpr := buildKeysetWhere(entityPkg, fields)
	methodName := string(method.Name())

	data := listTemplateData{
		StructName:     req.ServiceStructName,
		MethodName:     methodName,
		ReqPkgAlias:    reqPkgName,
		ReqType:        string(input.Name()),
		RespType:       string(output.Name()),
		EntityType:     entityShortName,
		RespListField:  goCamelCase(string(respListField.Name())),
		NextTokenField: goCamelCase(string(nextTokenField.Name())),
		ClientField:    req.Type.Name,
		EntityPkg:      entityPkg,
		Fields:         fields,
		FieldCount:     len(fields),
		WhereExpr:      whereExpr,
		SetFields:      setFields,
		DefaultConst:   methodName + "DefaultPageSize",
		DefaultValue:   listPageSizeDefault,
		MaxConst:       methodName + "MaxPageSize",
		MaxValue:       listPageSizeMax,
	}

	var buf bytes.Buffer
	if err := listTemplate.Execute(&buf, data); err != nil {
		return MethodImpl{}, fmt.Errorf("entconnect: %s.%s: render list.tmpl: %w", req.Type.Name, req.Binding.Op, err)
	}

	return MethodImpl{
		Body: buf.String(),
		Imports: []string{
			reqPkg,
			"context",
			"fmt",
			"strconv",
			"time",
			"connectrpc.com/connect",
			"google.golang.org/protobuf/types/known/timestamppb",
		},
	}, nil
}

// listOrderFields builds the ordering tuple: every Binding.OrderBy entry
// (a proto field name, mapped to its derived ent field via
// mixinforproto.SourceField.FieldName — D-09's schema-side ordering
// channel), followed by the entity's primary key, always last, so the
// tuple is total and unique by construction (CRUD-03's tie-breaking
// guarantee). Iterates OrderBy in its declared (already schema-author-
// ordered) order, never map order (D-20).
func listOrderFields(req GenRequest) ([]listFieldSpec, error) {
	byProtoName := make(map[string]*gen.Field, len(req.Type.Fields))
	for _, f := range req.Type.Fields {
		sf, err := decodeAnnotation[mixinforproto.SourceField](f.Annotations, mixinforproto.MixinForProtoField)
		if err != nil {
			// No SourceField provenance on this field (a hand-declared
			// override, or a field predating mixinforproto's derivation) —
			// ListRPC's orderBy can only name derived fields.
			continue
		}
		byProtoName[sf.FieldName] = f
	}

	seenID := false
	specs := make([]listFieldSpec, 0, len(req.Binding.OrderBy)+1)
	for _, protoName := range req.Binding.OrderBy {
		f, ok := byProtoName[protoName]
		if !ok {
			return nil, fmt.Errorf(
				"entconnect: %s.%s: ListRPC orderBy names %q, which has no derived ent field on %q (excluded, overridden, or not a proto field on the bound entity)",
				req.Type.Name, req.Binding.Op, protoName, req.SourceMessage.Message,
			)
		}
		goType := ""
		if f.Type != nil {
			goType = f.Type.String()
		}
		goName := goCamelCase(f.Name)
		spec, err := newListFieldSpec(goName, goType, "row."+goName)
		if err != nil {
			return nil, fmt.Errorf("entconnect: %s.%s: orderBy field %q: %w", req.Type.Name, req.Binding.Op, protoName, err)
		}
		specs = append(specs, spec)
		if goName == "ID" {
			seenID = true
		}
	}

	if !seenID {
		idGoType := "int"
		if req.Type.ID != nil && req.Type.ID.Type != nil {
			idGoType = req.Type.ID.Type.String()
		}
		spec, err := newListFieldSpec("ID", idGoType, "row.ID")
		if err != nil {
			return nil, fmt.Errorf("entconnect: %s.%s: %w", req.Type.Name, req.Binding.Op, err)
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func newListFieldSpec(goName, goType, valueExpr string) (listFieldSpec, error) {
	kind, err := classifyListGoType(goType)
	if err != nil {
		return listFieldSpec{}, err
	}
	spec := listFieldSpec{
		GoName:    goName,
		GoType:    goType,
		ValueExpr: valueExpr,
		VarName:   "key" + goName,
		GTFunc:    goName + "GT",
		EQFunc:    goName + "EQ",
		OrderFunc: "By" + goName,
	}
	switch kind {
	case listKindTime:
		spec.EncodeExpr = "last." + goName + ".Format(time.RFC3339Nano)"
	case listKindInt:
		spec.EncodeExpr = "strconv.FormatInt(int64(last." + goName + "), 10)"
	default: // listKindString
		spec.EncodeExpr = "last." + goName
	}
	return spec, nil
}

// decodeStmtFor renders the Go statement(s) that parse
// cursor.Keys[index] into a local variable named spec.VarName, or return
// a well-formed connect.CodeInvalidArgument error naming the offending
// field on a parse failure. spec.GoType was already validated by
// newListFieldSpec (classifyListGoType), so the switch below cannot hit
// its default case in practice — it exists only to keep this function
// total rather than assume the caller never changes that invariant.
func decodeStmtFor(spec listFieldSpec, index int) string {
	kind, err := classifyListGoType(spec.GoType)
	if err != nil {
		// Unreachable in practice (newListFieldSpec already validated the
		// type); fall through to the string case, the least surprising
		// default, rather than silently drop the field.
		kind = listKindString
	}
	switch kind {
	case listKindTime:
		return fmt.Sprintf(
			"%s, err := time.Parse(time.RFC3339Nano, cursor.Keys[%d])\n\t\tif err != nil {\n\t\t\treturn nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf(\"page_token: invalid %s: %%w\", err))\n\t\t}",
			spec.VarName, index, spec.GoName,
		)
	case listKindInt:
		return fmt.Sprintf(
			"%sParsed, err := strconv.ParseInt(cursor.Keys[%d], 10, 64)\n\t\tif err != nil {\n\t\t\treturn nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf(\"page_token: invalid %s: %%w\", err))\n\t\t}\n\t\t%s := %s(%sParsed)",
			spec.VarName, index, spec.GoName, spec.VarName, spec.GoType, spec.VarName,
		)
	default: // listKindString
		return fmt.Sprintf("%s := cursor.Keys[%d]", spec.VarName, index)
	}
}

// buildKeysetWhere renders the standard lexicographic keyset disjunction
// over fields (RESEARCH.md Pattern 2): for a tuple (a, b, id) this is
// Or(aGT, And(aEQ, bGT), And(aEQ, bEQ, idGT)) — using entityPkg-qualified
// GT/EQ predicate functions and the entityPkg-qualified And/Or
// combinators. All ascending: GT (never LT), because the ordering tuple
// is rendered ascending (list.tmpl's Order call).
func buildKeysetWhere(entityPkg string, fields []listFieldSpec) string {
	n := len(fields)
	terms := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts := make([]string, 0, i+1)
		for j := 0; j < i; j++ {
			parts = append(parts, fmt.Sprintf("%s.%s(%s)", entityPkg, fields[j].EQFunc, fields[j].VarName))
		}
		parts = append(parts, fmt.Sprintf("%s.%s(%s)", entityPkg, fields[i].GTFunc, fields[i].VarName))
		if len(parts) == 1 {
			terms = append(terms, parts[0])
		} else {
			terms = append(terms, fmt.Sprintf("%s.And(\n\t\t\t%s,\n\t\t)", entityPkg, strings.Join(parts, ",\n\t\t\t")))
		}
	}
	if len(terms) == 1 {
		return terms[0]
	}
	return fmt.Sprintf("%s.Or(\n\t\t%s,\n\t)", entityPkg, strings.Join(terms, ",\n\t\t"))
}
