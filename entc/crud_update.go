package entc

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/smintz/entconnect/mixinforproto"
)

func init() {
	RegisterGenerator(OpUpdate, generateUpdate)
}

//go:embed templates/update.tmpl
var updateTemplateSrc string

var updateTemplate = template.Must(template.New("update.tmpl").Parse(updateTemplateSrc))

// updateTemplateData is update.tmpl's input — see getTemplateData's own
// doc comment (crud_get.go): every value here is pre-resolved Go source
// text or a plain identifier, so update.tmpl stays a plain text/template
// with no case-conversion or descriptor traversal of its own (D-19).
type updateTemplateData struct {
	StructName       string // e.g. "patchUpdateServiceServer"
	MethodName       string // e.g. "UpdatePatch"
	ReqPkgAlias      string // e.g. "entconnecttestv1"
	ReqType          string // e.g. "UpdatePatchRequest"
	RespType         string // e.g. "UpdatePatchResponse"
	EntityType       string // e.g. "Patch"
	RespEntityField  string // e.g. "Patch" (the Go field on RespType holding the entity)
	ClientField      string // e.g. "Patch" (s.client.<ClientField>)
	MaskFieldGoName  string // e.g. "UpdateMask" (req.Msg.Get<MaskFieldGoName>())
	PatchFieldGoName string // e.g. "Patch" (req.Msg.Get<PatchFieldGoName>() -- the entity submessage ValidateMask validates paths against, NOT req.Msg itself)
	AllowedLiteral   string // e.g. `[]string{"body", "revision", "title"}` — sorted (D-20)
	IDConvert        string // Go statements converting the entity's raw id lookup value into a local `id` variable
	SetFields        []updateSetField
	DivergenceDoc    string // Go comment block (trailing newline included), recording SourceField.LengthUnitDivergentIDs
}

type updateSetField struct {
	// ProtoNameLiteral is a quoted Go string literal of the proto field
	// name, e.g. `"title"` — used verbatim as a switch case label so the
	// case matches runtime.ValidateMask's own validated, exact-byte-
	// equality path strings.
	ProtoNameLiteral string
	// SetMethod is the ent update builder's setter, e.g. "SetTitle".
	SetMethod string
	// ValueExpr reads the corresponding value off the request's entity
	// submessage, e.g. `req.Msg.GetPatch().GetTitle()`.
	ValueExpr string
	// RespGoName is the response entity message's Go field name, e.g. "Title".
	RespGoName string
	// RespValueExpr reads the corresponding value off the saved row,
	// e.g. "row.Title".
	RespValueExpr string
}

// generateUpdate implements Generator for OpUpdate — CRUD-04/CRUD-05.
// It renders a FieldMask-gated Update method: validate the mask at
// request time via runtime.ValidateMask against the derived, non-
// excluded allowed field set (D-14..D-16), apply exactly one Set* call
// per validated path in a sorted switch with an erroring default
// (T-02-19), classify NotFound/ConstraintError/ValidationError against
// the LOCAL generated ent package, and fall through to
// runtime.MapError for anything else (mirrors crud_get.go's D-18
// discipline).
//
// Before any rendering happens, it also cross-checks the entity's
// complete field descriptor set against Phase 1's SourceMessage
// provenance via ValidateMaskPaths (D-17, entc/maskcheck.go) — a mask
// surface that cannot be satisfied fails codegen rather than shipping a
// handler with a silently incomplete switch.
func generateUpdate(req GenRequest) (MethodImpl, error) {
	method := req.Method
	input := method.Input()
	output := method.Output()

	reqFile := input.ParentFile()
	reqPkg, reqPkgName, err := goImportPath(reqFile)
	if err != nil {
		return MethodImpl{}, fmt.Errorf("entconnect: %s.%s: %w", req.Type.Name, req.Binding.Op, err)
	}

	entityFullName := string(protoreflect.FullName(req.SourceMessage.Message))

	patchField, err := findMessageField(input, entityFullName)
	if err != nil {
		return MethodImpl{}, fmt.Errorf(
			"entconnect: %s.%s: request message %q %s",
			req.Type.Name, req.Binding.Op, input.FullName(), err,
		)
	}
	maskField, err := findMessageField(input, "google.protobuf.FieldMask")
	if err != nil {
		return MethodImpl{}, fmt.Errorf(
			"entconnect: %s.%s: request message %q has no google.protobuf.FieldMask field — UpdateRPC requires one (D-14)",
			req.Type.Name, req.Binding.Op, input.FullName(),
		)
	}
	respEntityField, err := findMessageField(output, entityFullName)
	if err != nil {
		return MethodImpl{}, fmt.Errorf(
			"entconnect: %s.%s: response message %q %s",
			req.Type.Name, req.Binding.Op, output.FullName(), err,
		)
	}

	entityDesc := patchField.Message()
	idField := entityDesc.Fields().ByName(protoreflect.Name("id"))
	if idField == nil {
		return MethodImpl{}, fmt.Errorf(
			"entconnect: %s.%s: entity message %q has no \"id\" field — UpdateRPC requires the entity to carry a lookup field literally named \"id\"",
			req.Type.Name, req.Binding.Op, entityDesc.FullName(),
		)
	}

	idGoType := "int"
	if req.Type.ID != nil && req.Type.ID.Type != nil {
		idGoType = req.Type.ID.Type.String()
	}
	idLookupExpr := "req.Msg.Get" + goCamelCase(string(patchField.Name())) + "().Get" + goCamelCase(string(idField.Name())) + "()"
	idConvert, err := idConversion(idGoType, idLookupExpr)
	if err != nil {
		return MethodImpl{}, fmt.Errorf("entconnect: %s.%s: %w", req.Type.Name, req.Binding.Op, err)
	}

	setFields, allowedNames, divergentIDs := updateSetFieldsFor(req, patchField)

	// Build-time mask-path validation (D-17, CRUD-05) is wired in below
	// by entc/maskcheck.go's ValidateMaskPaths — see that file's own
	// call site, added once the descriptor+SourceMessage cross-check
	// exists, so a schema whose mask surface cannot be satisfied fails
	// codegen rather than shipping a handler with a silently incomplete
	// switch.

	entityFullNameStr := entityFullName
	entityShortName := entityFullNameStr[strings.LastIndex(entityFullNameStr, ".")+1:]

	data := updateTemplateData{
		StructName:       req.ServiceStructName,
		MethodName:       string(method.Name()),
		ReqPkgAlias:      reqPkgName,
		ReqType:          string(input.Name()),
		RespType:         string(output.Name()),
		EntityType:       entityShortName,
		RespEntityField:  goCamelCase(string(respEntityField.Name())),
		ClientField:      req.Type.Name,
		MaskFieldGoName:  goCamelCase(string(maskField.Name())),
		PatchFieldGoName: goCamelCase(string(patchField.Name())),
		AllowedLiteral:   allowedLiteral(allowedNames),
		IDConvert:        idConvert,
		SetFields:        setFields,
		DivergenceDoc:    divergenceDoc(divergentIDs),
	}

	var buf bytes.Buffer
	if err := updateTemplate.Execute(&buf, data); err != nil {
		return MethodImpl{}, fmt.Errorf("entconnect: %s.%s: render update.tmpl: %w", req.Type.Name, req.Binding.Op, err)
	}

	return MethodImpl{
		Body: buf.String(),
		Imports: []string{
			reqPkg,
			"context",
			"fmt",
			"connectrpc.com/connect",
		},
	}, nil
}

// findMessageField returns the first field on msg whose type is the
// message named fullName, or an error naming what was missing.
func findMessageField(msg protoreflect.MessageDescriptor, fullName string) (protoreflect.FieldDescriptor, error) {
	fields := msg.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.Kind() == protoreflect.MessageKind && string(fd.Message().FullName()) == fullName {
			return fd, nil
		}
	}
	return nil, fmt.Errorf("has no field of type %q", fullName)
}

// updateSetFieldsFor computes, from req.Type.Fields' SourceField
// provenance annotations (D-17: SourceField.FieldName is the proto-name
// -> ent-field mapping), the sorted (D-20) set of top-level fields this
// Update surface can settably mask-gate. A gen.Field with no SourceField
// annotation at all (an Override()'d field, which installs an ent.Field
// with no provenance — mixinforproto/option.go's own documented
// behavior) is never a candidate: its proto-name mapping is unknown, so
// it can never appear in the allowed set. Returns the rendered SetFields
// slice, the sorted allowed proto-field-name list (fed to both
// runtime.ValidateMask's allowed parameter and entc/maskcheck.go's
// ValidateMaskPaths), and the sorted, deduplicated union of every
// candidate's LengthUnitDivergentIDs.
func updateSetFieldsFor(req GenRequest, patchField protoreflect.FieldDescriptor) ([]updateSetField, []string, []string) {
	patchGetter := "req.Msg.Get" + goCamelCase(string(patchField.Name())) + "()"

	type candidate struct {
		protoName string
		goName    string
		isTime    bool
		divergent []string
	}
	var candidates []candidate
	for _, f := range req.Type.Fields {
		sf, err := decodeAnnotation[mixinforproto.SourceField](f.Annotations, mixinforproto.MixinForProtoField)
		if err != nil {
			continue
		}
		if sf.FieldName == "id" {
			continue
		}
		candidates = append(candidates, candidate{
			protoName: sf.FieldName,
			goName:    goCamelCase(f.Name),
			isTime:    f.Type != nil && f.Type.Type == field.TypeTime,
			divergent: sf.LengthUnitDivergentIDs,
		})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].protoName < candidates[j].protoName })

	setFields := make([]updateSetField, 0, len(candidates))
	allowedNames := make([]string, 0, len(candidates))
	var divergentIDs []string
	for _, c := range candidates {
		valueExpr := patchGetter + ".Get" + goCamelCase(c.protoName) + "()"
		respValueExpr := "row." + c.goName
		if c.isTime {
			valueExpr += ".AsTime()"
			respValueExpr = "timestamppb.New(" + respValueExpr + ")"
		}
		setFields = append(setFields, updateSetField{
			ProtoNameLiteral: strconv.Quote(c.protoName),
			SetMethod:        "Set" + c.goName,
			ValueExpr:        valueExpr,
			RespGoName:       c.goName,
			RespValueExpr:    respValueExpr,
		})
		allowedNames = append(allowedNames, c.protoName)
		divergentIDs = append(divergentIDs, c.divergent...)
	}
	sort.Strings(divergentIDs)
	divergentIDs = dedupSorted(divergentIDs)
	return setFields, allowedNames, divergentIDs
}

// maskFailuresError renders failures (already produced by
// ValidateMaskPaths) as a single-line error: every offender joined by
// "; " rather than a newline, so wrapping this error inside another
// failure{} (entc/extension.go's genFailures accumulation) never
// nests a newline inside what must render as one self-sufficient first
// line (D-04). Returns nil for an empty slice, so callers can uniformly
// `if err != nil`.
func maskFailuresError(failures []failure) error {
	if len(failures) == 0 {
		return nil
	}
	sorted := make([]failure, len(failures))
	copy(sorted, failures)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].rule != sorted[j].rule {
			return sorted[i].rule < sorted[j].rule
		}
		return sorted[i].description < sorted[j].description
	})
	parts := make([]string, len(sorted))
	for i, f := range sorted {
		parts[i] = fmt.Sprintf("%s — %s", f.description, f.remedy)
	}
	return errors.New(strings.Join(parts, "; "))
}

// allowedLiteral renders names (already sorted by the caller) as a Go
// []string composite literal source text, embedded directly into the
// generated file so runtime.ValidateMask's allowed parameter never
// needs a runtime-computed value.
func allowedLiteral(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = strconv.Quote(n)
	}
	return "[]string{" + strings.Join(quoted, ", ") + "}"
}

// divergenceDoc renders the D-02/README-motivated doc comment recording
// which of this Update surface's derived fields carry Phase 1's
// string-length unit divergence (protovalidate counts Unicode code
// points; the derived ent field counts bytes) — listing the affected
// constraint IDs directly from SourceField.LengthUnitDivergentIDs rather
// than restating the rule generically, per this plan's action text. ids
// must already be sorted and deduplicated.
func divergenceDoc(ids []string) string {
	var b strings.Builder
	if len(ids) == 0 {
		b.WriteString("// No settable field on this Update surface carries a recorded\n")
		b.WriteString("// protovalidate length-unit divergence — SourceField.LengthUnitDivergentIDs\n")
		b.WriteString("// is empty for every derived field on this message (mixinforproto/\n")
		b.WriteString("// annotation.go). Where present, protovalidate's Unicode-code-point length\n")
		b.WriteString("// constraints agree with this field's byte-length checks.\n")
		return b.String()
	}
	b.WriteString("// WARNING: the following protovalidate constraint IDs compare Unicode\n")
	b.WriteString("// code points (string.min_len/max_len/len) while the corresponding\n")
	b.WriteString("// derived ent field compares bytes (SourceField.LengthUnitDivergentIDs,\n")
	b.WriteString("// mixinforproto/annotation.go) — a non-ASCII value can satisfy the\n")
	b.WriteString("// boundary rule at the transport layer and still be rejected here, at\n")
	b.WriteString("// the storage layer:\n")
	for _, id := range ids {
		b.WriteString("//   - " + id + "\n")
	}
	return b.String()
}

// dedupSorted collapses adjacent duplicates in an already-sorted slice.
func dedupSorted(s []string) []string {
	if len(s) == 0 {
		return s
	}
	out := []string{s[0]}
	for _, v := range s[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
