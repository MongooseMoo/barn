// Package wasmbuiltins hosts pure WIT components. Parsing and schema planning
// happen at load time; ordinary native builtin dispatch does not use this code.
package wasmbuiltins

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/MongooseMoo/barn/types"
	"github.com/samyfodil/wazy/component"
	"go.bytecodealliance.org/wit"
)

const maxNodes = 65536
const maxValueBytes = 1 << 20

type shape struct {
	name    string
	bits    int
	element *shape
	fields  []field
	cases   []field
	dynamic int
	nodes   int
}

type field struct {
	name  string
	shape *shape
}
type function struct {
	name   string
	params []field
	result *shape
}

func plan(t any, depth int) (*shape, error) {
	if depth > 16 {
		return nil, fmt.Errorf("WIT type nesting exceeds 16")
	}
	s := &shape{}
	switch t := t.(type) {
	case nil:
		s.name = "unit"
	case *wit.TypeDef:
		return plan(t.Kind, depth+1)
	case wit.Bool:
		s.name = "bool"
	case wit.S8:
		s.name, s.bits = "signed", 8
	case wit.S16:
		s.name, s.bits = "signed", 16
	case wit.S32:
		s.name, s.bits = "signed", 32
	case wit.S64:
		s.name, s.bits = "signed", 64
	case wit.U8:
		s.name, s.bits = "unsigned", 8
	case wit.U16:
		s.name, s.bits = "unsigned", 16
	case wit.U32:
		s.name, s.bits = "unsigned", 32
	case wit.U64:
		s.name, s.bits = "unsigned", 64
	case wit.F32:
		s.name, s.bits = "float", 32
	case wit.F64:
		s.name, s.bits = "float", 64
	case wit.Char:
		s.name = "char"
	case wit.String:
		s.name, s.dynamic = "string", 1
	case *wit.List:
		e, err := plan(t.Type, depth+1)
		if err != nil {
			return nil, err
		}
		// Wazy's scalar-list path checks both multiplication and addition before
		// allocating. Composite-list lifting does not yet provide that guarantee.
		if e.name != "signed" && e.name != "unsigned" && e.name != "float" && e.name != "bool" && e.name != "char" {
			return nil, fmt.Errorf("list elements must be fixed-width WIT primitives")
		}
		s.name, s.element, s.dynamic = "list", e, 1
	case *wit.Record:
		s.name = "record"
		for _, f := range t.Fields {
			e, err := plan(f.Type, depth+1)
			if err != nil {
				return nil, err
			}
			s.fields = append(s.fields, field{f.Name, e})
			s.dynamic += e.dynamic
		}
	case *wit.Tuple:
		s.name = "tuple"
		for _, t := range t.Types {
			e, err := plan(t, depth+1)
			if err != nil {
				return nil, err
			}
			s.fields = append(s.fields, field{"", e})
			s.dynamic += e.dynamic
		}
	case *wit.Option:
		e, err := plan(t.Type, depth+1)
		if err != nil {
			return nil, err
		}
		s.name, s.element, s.dynamic = "option", e, e.dynamic
		if e.name == "option" {
			return nil, fmt.Errorf("nested options are not supported by the component runtime")
		}
	case *wit.Result:
		s.name = "result"
		for i, t := range []wit.Type{t.OK, t.Err} {
			e, err := plan(t, depth+1)
			if err != nil {
				return nil, err
			}
			s.cases = append(s.cases, field{[]string{"ok", "err"}[i], e})
			s.dynamic = max(s.dynamic, e.dynamic)
		}
	case *wit.Enum:
		s.name = "enum"
		for _, c := range t.Cases {
			s.cases = append(s.cases, field{c.Name, nil})
		}
	case *wit.Variant:
		s.name = "variant"
		for _, c := range t.Cases {
			e, err := plan(c.Type, depth+1)
			if err != nil {
				return nil, err
			}
			s.cases = append(s.cases, field{c.Name, e})
			s.dynamic = max(s.dynamic, e.dynamic)
		}
	default:
		return nil, fmt.Errorf("unsupported WIT type %T (resources, flags and async types are not supported)", t)
	}
	s.nodes = 1
	if s.element != nil {
		s.nodes += s.element.nodes
	}
	for _, f := range s.fields {
		s.nodes += f.shape.nodes
	}
	for _, c := range s.cases {
		if c.shape != nil {
			s.nodes += c.shape.nodes
		}
	}
	if len(s.fields) > 64 || len(s.cases) > 64 || s.dynamic > 4 || s.nodes > 1024 {
		return nil, fmt.Errorf("WIT type exceeds the component value budget")
	}
	return s, nil
}

func (s *shape) String() string {
	if s.name == "signed" {
		return fmt.Sprintf("s%d", s.bits)
	}
	if s.name == "unsigned" {
		return fmt.Sprintf("u%d", s.bits)
	}
	if s.name == "float" {
		return fmt.Sprintf("f%d", s.bits)
	}
	if s.element != nil {
		return s.name + "<" + s.element.String() + ">"
	}
	if len(s.fields) > 0 {
		var parts []string
		for _, f := range s.fields {
			parts = append(parts, f.name+":"+f.shape.String())
		}
		return s.name + "{" + strings.Join(parts, ",") + "}"
	}
	if len(s.cases) > 0 {
		parts := make([]string, 0, len(s.cases))
		for _, c := range s.cases {
			part := c.name
			if c.shape != nil && c.shape.name != "unit" {
				part += "(" + c.shape.String() + ")"
			}
			parts = append(parts, part)
		}
		return s.name + "{" + strings.Join(parts, ",") + "}"
	}
	return s.name
}

type budget struct{ nodes, bytes int }

func (b *budget) take(bytes int) types.ErrorCode {
	b.nodes++
	b.bytes += bytes
	if b.nodes > maxNodes || b.bytes > maxValueBytes {
		return types.E_QUOTA
	}
	return types.E_NONE
}

func (s *shape) lower(v types.Value, b *budget) (any, types.ErrorCode) {
	if e := b.take(0); e != types.E_NONE {
		return nil, e
	}
	switch s.name {
	case "unit":
		if v.Type() != types.TYPE_INT || v.Int() != 0 {
			return nil, types.E_TYPE
		}
		return nil, types.E_NONE
	case "signed", "unsigned":
		if v.Type() != types.TYPE_INT {
			return nil, types.E_TYPE
		}
		n := v.Int()
		if s.name == "unsigned" {
			if n < 0 || (s.bits < 64 && uint64(n) >= uint64(1)<<s.bits) {
				return nil, types.E_RANGE
			}
			if s.bits == 64 {
				return uint64(n), types.E_NONE
			}
			return uint32(n), types.E_NONE
		}
		if s.bits < 64 && (n < -(int64(1)<<(s.bits-1)) || n >= (int64(1)<<(s.bits-1))) {
			return nil, types.E_RANGE
		}
		if s.bits == 64 {
			return n, types.E_NONE
		}
		return int32(n), types.E_NONE
	case "float":
		if v.Type() != types.TYPE_FLOAT {
			return nil, types.E_TYPE
		}
		if math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0) {
			return nil, types.E_FLOAT
		}
		if s.bits == 32 {
			n := float32(v.Float())
			if math.IsInf(float64(n), 0) {
				return nil, types.E_FLOAT
			}
			return n, types.E_NONE
		}
		return v.Float(), types.E_NONE
	case "bool":
		if v.Type() != types.TYPE_BOOL {
			return nil, types.E_TYPE
		}
		return v.Bool(), types.E_NONE
	case "string", "char":
		if v.Type() != types.TYPE_STR {
			return nil, types.E_TYPE
		}
		str := v.Str()
		if !utf8.ValidString(str) {
			return nil, types.E_INVARG
		}
		if e := b.take(len(str)); e != types.E_NONE {
			return nil, e
		}
		if s.name == "char" {
			if utf8.RuneCountInString(str) != 1 {
				return nil, types.E_INVARG
			}
			r, _ := utf8.DecodeRuneInString(str)
			return r, types.E_NONE
		}
		return str, types.E_NONE
	case "list", "tuple", "option":
		if v.Type() != types.TYPE_LIST {
			return nil, types.E_TYPE
		}
		if v.Len() > maxNodes {
			return nil, types.E_QUOTA
		}
		if s.name == "option" {
			if v.Len() == 0 {
				return nil, types.E_NONE
			}
			if v.Len() != 1 {
				return nil, types.E_INVARG
			}
			return s.element.lower(v.Get(1), b)
		}
		if s.name == "tuple" && v.Len() != len(s.fields) {
			return nil, types.E_INVARG
		}
		out := make([]any, v.Len())
		for i, val := range v.Elements() {
			e := s.element
			if s.name == "tuple" {
				e = s.fields[i].shape
			}
			x, err := e.lower(val, b)
			if err != types.E_NONE {
				return nil, err
			}
			out[i] = x
		}
		return out, types.E_NONE
	case "record":
		if v.Type() != types.TYPE_MAP {
			return nil, types.E_TYPE
		}
		if v.Len() != len(s.fields) {
			return nil, types.E_INVARG
		}
		out := make([]any, len(s.fields))
		for i, f := range s.fields {
			val, ok := v.GetWithCase(types.NewStr(f.name), true)
			if !ok {
				return nil, types.E_INVARG
			}
			x, e := f.shape.lower(val, b)
			if e != types.E_NONE {
				return nil, e
			}
			out[i] = x
		}
		return out, types.E_NONE
	case "enum", "variant", "result":
		name := v
		payload := types.NewInt(0)
		if s.name != "enum" {
			if v.Type() != types.TYPE_LIST {
				return nil, types.E_TYPE
			}
			if v.Len() != 2 {
				return nil, types.E_INVARG
			}
			name, payload = v.Get(1), v.Get(2)
		}
		if name.Type() != types.TYPE_STR {
			return nil, types.E_TYPE
		}
		for i, c := range s.cases {
			if c.name == name.Str() {
				if s.name == "enum" {
					return uint32(i), types.E_NONE
				}
				p, e := c.shape.lower(payload, b)
				if e != types.E_NONE {
					return nil, e
				}
				if s.name == "result" {
					return component.ResultValue{IsErr: i == 1, Payload: p}, types.E_NONE
				}
				return component.VariantValue{Disc: uint32(i), Payload: p}, types.E_NONE
			}
		}
		return nil, types.E_INVARG
	}
	return nil, types.E_INVARG
}

func (s *shape) lift(v any, b *budget) (types.Value, types.ErrorCode) {
	bad := func(e types.ErrorCode) (types.Value, types.ErrorCode) { return types.None, e }
	if e := b.take(0); e != types.E_NONE {
		return bad(e)
	}
	switch s.name {
	case "unit":
		return types.NewInt(0), types.E_NONE
	case "signed", "unsigned":
		r := reflect.ValueOf(v)
		var n int64
		if r.Kind() >= reflect.Int && r.Kind() <= reflect.Int64 {
			n = r.Int()
		} else if r.Kind() >= reflect.Uint && r.Kind() <= reflect.Uint64 {
			if r.Uint() > math.MaxInt64 {
				return bad(types.E_RANGE)
			}
			n = int64(r.Uint())
		} else {
			return bad(types.E_INVARG)
		}
		return types.NewInt(n), types.E_NONE
	case "float":
		r := reflect.ValueOf(v)
		if r.Kind() != reflect.Float32 && r.Kind() != reflect.Float64 {
			return bad(types.E_INVARG)
		}
		if math.IsNaN(r.Float()) || math.IsInf(r.Float(), 0) {
			return bad(types.E_FLOAT)
		}
		return types.NewFloat(r.Float()), types.E_NONE
	case "bool":
		x, ok := v.(bool)
		if !ok {
			return bad(types.E_INVARG)
		}
		return types.NewBool(x), types.E_NONE
	case "string":
		x, ok := v.(string)
		if !ok {
			return bad(types.E_INVARG)
		}
		if e := b.take(len(x)); e != types.E_NONE {
			return bad(e)
		}
		return types.NewStr(x), types.E_NONE
	case "char":
		x, ok := v.(rune)
		if !ok || !utf8.ValidRune(x) {
			return bad(types.E_INVARG)
		}
		return types.NewStr(string(x)), types.E_NONE
	case "list", "tuple", "record":
		r := reflect.ValueOf(v)
		if r.Kind() != reflect.Slice {
			return bad(types.E_INVARG)
		}
		if r.Len() > maxNodes {
			return bad(types.E_QUOTA)
		}
		if s.name != "list" && r.Len() != len(s.fields) {
			return bad(types.E_INVARG)
		}
		out := make([]types.Value, r.Len())
		for i := range out {
			e := s.element
			if s.name != "list" {
				e = s.fields[i].shape
			}
			x, err := e.lift(r.Index(i).Interface(), b)
			if err != types.E_NONE {
				return bad(err)
			}
			out[i] = x
		}
		if s.name == "record" {
			pairs := make([][2]types.Value, len(out))
			for i, x := range out {
				pairs[i] = [2]types.Value{types.NewStr(s.fields[i].name), x}
			}
			return types.NewMap(pairs), types.E_NONE
		}
		return types.NewList(out), types.E_NONE
	case "option":
		if v == nil {
			return types.NewEmptyList(), types.E_NONE
		}
		x, e := s.element.lift(v, b)
		if e != types.E_NONE {
			return bad(e)
		}
		return types.NewList([]types.Value{x}), types.E_NONE
	case "enum", "variant", "result":
		var disc uint32
		var payload any
		switch s.name {
		case "enum":
			x, ok := v.(uint32)
			if !ok {
				return bad(types.E_INVARG)
			}
			disc = x
		case "variant":
			x, ok := v.(component.VariantValue)
			if !ok {
				return bad(types.E_INVARG)
			}
			disc, payload = x.Disc, x.Payload
		case "result":
			x, ok := v.(component.ResultValue)
			if !ok {
				return bad(types.E_INVARG)
			}
			if x.IsErr {
				disc = 1
			}
			payload = x.Payload
		}
		if int(disc) >= len(s.cases) {
			return bad(types.E_INVARG)
		}
		c := s.cases[disc]
		if s.name == "enum" {
			return types.NewStr(c.name), types.E_NONE
		}
		x, e := c.shape.lift(payload, b)
		if e != types.E_NONE {
			return bad(e)
		}
		return types.NewList([]types.Value{types.NewStr(c.name), x}), types.E_NONE
	}
	return bad(types.E_INVARG)
}
