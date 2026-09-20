//go:build geas

package geas

/*
#include <stdlib.h>
#include <string.h>
#include <geas/geas.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Tag is a value type tag
type Tag uint32

// The type tags that cross the boundary
const (
	TagUnit Tag = iota
	TagInt
	TagUInt
	TagFloat
	TagBool
	TagByte
	TagChar
	TagString
	TagList
	TagMap
	TagTuple
	TagOption
	TagResult
	TagRecord
	TagPledgeRef
	TagSum
)

// Value is any Go value the codec carries, a scalar, string, or one of the shapes below
type Value interface{}

// Unit is the empty value
type Unit struct{}

// List is a typed list
type List struct {
	// The element tag
	Elem Tag
	// The elements
	Items []Value
}

// Tuple is a fixed set of values
type Tuple []Value

// Record is a record's fields in declaration order
type Record struct {
	// The fields
	Fields []Value
}

// Sum is one variant of a declared sum
type Sum struct {
	// The variant's declaration index
	Tag int
	// The payload fields in declaration order
	Fields []Value
}

// Option is Some or None
type Option struct {
	// Whether a value is present
	Some bool
	// The value when present
	Value Value
}

// Result is Ok or Err
type Result struct {
	// Whether the value is Ok
	Ok bool
	// The payload
	Value Value
}

// Map is an insertion ordered map with scalar keys
type Map struct {
	// The key tag
	Key Tag
	// The keys in insertion order
	Keys []Value
	// The values in the same order
	Values []Value
}

// valueSize is the byte size of one C value
const valueSize = unsafe.Sizeof(C.GeasValue{})

// mem hands out memory a value can point into
type mem interface {
	// Zeroed bytes
	bytes(n uintptr) unsafe.Pointer
	// A string value with its own copy of the bytes
	str(s string) C.GeasValue
	// A zeroed boxed value
	box() *C.GeasValue
}

// heapMem allocates with malloc and frees everything at once, for arguments the runtime copies
type heapMem struct {
	// Everything allocated so far
	ptrs []unsafe.Pointer
}

/**
 * bytes
 * Allocates zeroed bytes on the heap
 * @param n {uintptr} - the size
 * @return unsafe.Pointer
 **/
func (h *heapMem) bytes(n uintptr) unsafe.Pointer {
	// Never ask for zero
	if n == 0 {
		n = 1
	}
	p := C.calloc(1, C.size_t(n))
	h.ptrs = append(h.ptrs, p)
	return p
}

/**
 * str
 * Copies a string onto the heap as a string value
 * @param s {string} - the string
 * @return C.GeasValue
 **/
func (h *heapMem) str(s string) C.GeasValue {
	// The bytes
	p := h.bytes(uintptr(len(s)))
	if len(s) > 0 {
		C.memcpy(p, unsafe.Pointer(unsafe.StringData(s)), C.size_t(len(s)))
	}
	// The value
	var v C.GeasValue
	v.ty = C.GEAS_TY_STRING
	setArm(&v, C.GeasString{ptr: (*C.uint8_t)(p), len: C.uint64_t(len(s))})
	return v
}

/**
 * box
 * Allocates a zeroed value on the heap
 * @return *C.GeasValue
 **/
func (h *heapMem) box() *C.GeasValue {
	return (*C.GeasValue)(h.bytes(valueSize))
}

/**
 * free
 * Frees everything allocated so far
 * @return void
 **/
func (h *heapMem) free() {
	for _, p := range h.ptrs {
		C.free(p)
	}
	h.ptrs = nil
}

// instanceMem allocates on a contract instance, for values the instance must own
type instanceMem struct {
	// The instance
	c *C.GeasContract
}

/**
 * bytes
 * Allocates instance owned bytes
 * @param n {uintptr} - the size
 * @return unsafe.Pointer
 **/
func (m instanceMem) bytes(n uintptr) unsafe.Pointer {
	return unsafe.Pointer(C.geas_bytes(m.c, C.uint64_t(n)))
}

/**
 * str
 * Copies a string onto the instance
 * @param s {string} - the string
 * @return C.GeasValue
 **/
func (m instanceMem) str(s string) C.GeasValue {
	// An empty string needs no bytes
	if len(s) == 0 {
		return C.geas_string_copy(m.c, nil, 0)
	}
	return C.geas_string_copy(m.c, (*C.uint8_t)(unsafe.Pointer(unsafe.StringData(s))), C.uint64_t(len(s)))
}

/**
 * box
 * Allocates an instance owned boxed value
 * @return *C.GeasValue
 **/
func (m instanceMem) box() *C.GeasValue {
	return C.geas_box(m.c)
}

/**
 * getArm
 * Reads the union arm of a C value as a T by copying its bytes, safe whatever the alignment of the Go side struct
 * @param v {*C.GeasValue} - the value
 * @return T
 **/
func getArm[T any](v *C.GeasValue) T {
	// The result
	var x T
	// Copy the arm's leading bytes into it
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&x)), unsafe.Sizeof(x)), v.as[:])
	return x
}

/**
 * setArm
 * Writes a T into the union arm of a C value by copying its bytes
 * @param v {*C.GeasValue} - the value
 * @param x {T} - the arm content
 * @return void
 **/
func setArm[T any](v *C.GeasValue, x T) {
	// Copy the bytes over the arm
	copy(v.as[:], unsafe.Slice((*byte)(unsafe.Pointer(&x)), unsafe.Sizeof(x)))
}

/**
 * encode
 * Builds a C value from a Go value with memory from m
 * @param m {mem} - where the bytes come from
 * @param v {Value} - the Go value
 * @return C.GeasValue, error
 **/
func encode(m mem, v Value) (C.GeasValue, error) {
	// The value being built
	var out C.GeasValue
	// Dispatch on the Go type
	switch x := v.(type) {
	case nil, Unit:
		out.ty = C.GEAS_TY_UNIT
	case int:
		out.ty = C.GEAS_TY_INT
		setArm(&out, C.int64_t(x))
	case int64:
		out.ty = C.GEAS_TY_INT
		setArm(&out, C.int64_t(x))
	case uint64:
		out.ty = C.GEAS_TY_UINT
		setArm(&out, C.uint64_t(x))
	case float64:
		out.ty = C.GEAS_TY_FLOAT
		setArm(&out, C.double(x))
	case bool:
		out.ty = C.GEAS_TY_BOOL
		if x {
			setArm(&out, C.uint8_t(1))
		}
	case uint8:
		out.ty = C.GEAS_TY_BYTE
		setArm(&out, C.uint8_t(x))
	case rune:
		out.ty = C.GEAS_TY_CHAR
		setArm(&out, C.uint32_t(x))
	case string:
		out = m.str(x)
	case List:
		return encodeArm(m, C.GEAS_TY_LIST, 0, C.uint32_t(x.Elem), x.Items)
	case Tuple:
		return encodeArm(m, C.GEAS_TY_TUPLE, 0, 0, x)
	case Record:
		return encodeArm(m, C.GEAS_TY_RECORD, 0, 0, x.Fields)
	case Sum:
		return encodeArm(m, C.GEAS_TY_SUM, C.uint32_t(x.Tag), 0, x.Fields)
	case Map:
		return encodeMap(m, x)
	case Option:
		out.ty = C.GEAS_TY_OPTION
		if x.Some {
			out.tag = 1
			return boxed(m, out, x.Value)
		}
	case Result:
		out.ty = C.GEAS_TY_RESULT
		if !x.Ok {
			out.tag = 1
		}
		return boxed(m, out, x.Value)
	default:
		return out, fmt.Errorf("cannot encode %T", v)
	}
	return out, nil
}

/**
 * boxed
 * Encodes a payload into a fresh box and hangs it on a value
 * @param m {mem} - where the bytes come from
 * @param out {C.GeasValue} - the value with its tag set
 * @param payload {Value} - the payload
 * @return C.GeasValue, error
 **/
func boxed(m mem, out C.GeasValue, payload Value) (C.GeasValue, error) {
	// Encode the payload
	inner, err := encode(m, payload)
	if err != nil {
		return out, err
	}
	// Box it
	b := m.box()
	*b = inner
	setArm(&out, b)
	return out, nil
}

/**
 * encodeArm
 * Encodes values into a contiguous array behind the list arm
 * @param m {mem} - where the bytes come from
 * @param ty {C.uint32_t} - the type tag of the value
 * @param tag {C.uint32_t} - the variant tag for a sum
 * @param elem {C.uint32_t} - the element tag for a list
 * @param items {[]Value} - the elements
 * @return C.GeasValue, error
 **/
func encodeArm(m mem, ty, tag, elem C.uint32_t, items []Value) (C.GeasValue, error) {
	// The value
	var out C.GeasValue
	out.ty = ty
	out.tag = tag
	// The array, nil when empty
	var data unsafe.Pointer
	if len(items) > 0 {
		data = m.bytes(uintptr(len(items)) * valueSize)
		slots := unsafe.Slice((*C.GeasValue)(data), len(items))
		for i, it := range items {
			cv, err := encode(m, it)
			if err != nil {
				return out, err
			}
			slots[i] = cv
		}
	}
	// The arm
	setArm(&out, C.GeasList{data: data, len: C.uint64_t(len(items)), cap: C.uint64_t(len(items)), elem_ty: elem})
	return out, nil
}

/**
 * encodeMap
 * Encodes a map as interleaved pairs behind the list arm
 * @param m {mem} - where the bytes come from
 * @param x {Map} - the map
 * @return C.GeasValue, error
 **/
func encodeMap(m mem, x Map) (C.GeasValue, error) {
	// Refuse a ragged map
	if len(x.Keys) != len(x.Values) {
		return C.GeasValue{}, fmt.Errorf("map has %d keys and %d values", len(x.Keys), len(x.Values))
	}
	// Interleave
	pairs := make([]Value, 0, 2*len(x.Keys))
	for i := range x.Keys {
		pairs = append(pairs, x.Keys[i], x.Values[i])
	}
	return encodeArm(m, C.GEAS_TY_MAP, 0, C.uint32_t(x.Key), pairs)
}

/**
 * decode
 * Reads a C value into a Go value, copying every byte out of runtime memory
 * @param v {*C.GeasValue} - the value
 * @return Value
 **/
func decode(v *C.GeasValue) Value {
	// Nothing is unit
	if v == nil {
		return Unit{}
	}
	// Dispatch on the tag
	switch Tag(v.ty) {
	case TagUnit:
		return Unit{}
	case TagInt:
		return int64(getArm[C.int64_t](v))
	case TagUInt:
		return uint64(getArm[C.uint64_t](v))
	case TagFloat:
		return float64(getArm[C.double](v))
	case TagBool:
		return getArm[C.uint8_t](v) != 0
	case TagByte:
		return uint8(getArm[C.uint8_t](v))
	case TagChar:
		return rune(getArm[C.uint32_t](v))
	case TagString:
		s := getArm[C.GeasString](v)
		if s.ptr == nil || s.len == 0 {
			return ""
		}
		return C.GoStringN((*C.char)(unsafe.Pointer(s.ptr)), C.int(s.len))
	case TagList:
		return List{Elem: Tag(getArm[C.GeasList](v).elem_ty), Items: decodeArm(v)}
	case TagTuple:
		return Tuple(decodeArm(v))
	case TagRecord:
		return Record{Fields: decodeArm(v)}
	case TagSum:
		return Sum{Tag: int(v.tag), Fields: decodeArm(v)}
	case TagMap:
		pairs := decodeArm(v)
		m := Map{Key: Tag(getArm[C.GeasList](v).elem_ty)}
		for i := 0; i+1 < len(pairs); i += 2 {
			m.Keys = append(m.Keys, pairs[i])
			m.Values = append(m.Values, pairs[i+1])
		}
		return m
	case TagOption:
		if v.tag == 0 {
			return Option{}
		}
		return Option{Some: true, Value: decode(getArm[*C.GeasValue](v))}
	case TagResult:
		return Result{Ok: v.tag == 0, Value: decode(getArm[*C.GeasValue](v))}
	}
	// Anything else is opaque
	return Unit{}
}

/**
 * decodeArm
 * Reads every element behind the list arm
 * @param v {*C.GeasValue} - the value
 * @return []Value
 **/
func decodeArm(v *C.GeasValue) []Value {
	// The arm
	arm := getArm[C.GeasList](v)
	if arm.data == nil || arm.len == 0 {
		return nil
	}
	// The elements
	slots := unsafe.Slice((*C.GeasValue)(arm.data), int(arm.len))
	out := make([]Value, len(slots))
	for i := range slots {
		out[i] = decode(&slots[i])
	}
	return out
}
