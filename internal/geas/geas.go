//go:build geas

// Package geas hosts contracts from libgeasrt, the runtime behind service plugins
package geas

/*
#cgo LDFLAGS: -lgeasrt -lpthread
#include <stdlib.h>
#include <geas/geas.h>
#include "tramp.h"
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

// Status is a runtime status code
type Status int

// The pinned status numbers
const (
	OK Status = iota
	ErrState
	ErrType
	ErrVersion
	ErrUnbound
	ErrName
	ErrDeadlock
	ErrOOM
	ErrLoad
	ErrNet
	ErrStore
)

// statusNames spells every status
var statusNames = map[Status]string{
	OK: "ok", ErrState: "state", ErrType: "type", ErrVersion: "version", ErrUnbound: "unbound",
	ErrName: "name", ErrDeadlock: "deadlock", ErrOOM: "oom", ErrLoad: "load", ErrNet: "net", ErrStore: "store",
}

/**
 * Error
 * Spells the status as an error
 * @return string
 **/
func (s Status) Error() string {
	// Use the name when known
	if n, ok := statusNames[s]; ok {
		return "geas: " + n
	}
	return fmt.Sprintf("geas: status %d", int(s))
}

/**
 * check
 * Turns a C status into a Go error, nil on ok
 * @param st {C.GeasStatus} - the status
 * @return error
 **/
func check(st C.GeasStatus) error {
	// Ok is no error
	if st == C.GEAS_OK {
		return nil
	}
	return Status(st)
}

// State is a contract instance state
type State int

// The instance states
const (
	Unsigned State = iota
	Signed
	Fulfilled
	Partial
	Broken
)

/**
 * String
 * Spells the state the way the runtime does
 * @return string
 **/
func (s State) String() string {
	return C.GoString(C.geas_state_name(C.GeasContractState(s)))
}

// PledgeFunc is a Go implementation of a pledge, it returns the whole pledge value such as a Result
type PledgeFunc func(inst *Instance, args []Value) (Value, error)

// slots holds every bound Go function by trampoline slot
var slots struct {
	// Guards the table
	mu sync.RWMutex
	// The functions by slot
	fns [C.SYMPHONY_TRAMP_SLOTS]PledgeFunc
	// The next free slot
	next int
}

// Runtime is one libgeasrt runtime and its worker pool
type Runtime struct {
	// The runtime
	rt *C.GeasRuntime
}

/**
 * Init
 * Brings up a runtime with the default pool
 * @return *Runtime, error
 **/
func Init() (*Runtime, error) {
	// The runtime out param
	var rt *C.GeasRuntime
	// Init with defaults
	if err := check(C.geas_runtime_init(nil, &rt)); err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}
	return &Runtime{rt: rt}, nil
}

/**
 * Load
 * Loads a compiled module
 * @param path {string} - the .so
 * @return error
 **/
func (r *Runtime) Load(path string) error {
	// The C path
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	// Load
	if err := check(C.geas_module_load(r.rt, cpath)); err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	return nil
}

/**
 * Bind
 * Binds a Go function to a pledge named Contract.pledge, taking one trampoline slot for the life of the process
 * @param pledge {string} - the pledge name
 * @param fn {PledgeFunc} - the implementation
 * @return error
 **/
func (r *Runtime) Bind(pledge string, fn PledgeFunc) error {
	// Take a slot
	slots.mu.Lock()
	if slots.next >= len(slots.fns) {
		slots.mu.Unlock()
		return fmt.Errorf("bind %s: no trampoline slots left", pledge)
	}
	slot := slots.next
	slots.next++
	slots.fns[slot] = fn
	slots.mu.Unlock()
	// The C name
	cname := C.CString(pledge)
	defer C.free(unsafe.Pointer(cname))
	// Bind the slot's trampoline
	if err := check(C.geas_pledge_bind(r.rt, cname, C.symphony_tramp_at(C.int(slot)))); err != nil {
		return fmt.Errorf("bind %s: %w", pledge, err)
	}
	return nil
}

/**
 * BindC
 * Binds a C function pointer in the pledge frame shape straight to a pledge, no trampoline, for a body linked into the process such as a dusk archive
 * @param pledge {string} - the pledge name as Contract.pledge
 * @param fn {unsafe.Pointer} - the function
 * @return error
 **/
func (r *Runtime) BindC(pledge string, fn unsafe.Pointer) error {
	// Refuse a nil function
	if fn == nil {
		return fmt.Errorf("bind %s: nil function", pledge)
	}
	// The C name
	cname := C.CString(pledge)
	defer C.free(unsafe.Pointer(cname))
	// Bind it
	if err := check(C.geas_pledge_bind(r.rt, cname, C.GeasPledgeFn(fn))); err != nil {
		return fmt.Errorf("bind %s: %w", pledge, err)
	}
	return nil
}

/**
 * Freeze
 * Refuses every later load and bind
 * @return error
 **/
func (r *Runtime) Freeze() error {
	return check(C.geas_runtime_freeze(r.rt))
}

/**
 * Sign
 * Signs a contract with vow overrides
 * @param name {string} - the contract name
 * @param vows {map[string]Value} - the overrides, nil for defaults alone
 * @return *Instance, error
 **/
func (r *Runtime) Sign(name string, vows map[string]Value) (*Instance, error) {
	// The C name
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	// The bindings live on the heap until sign copies them
	h := &heapMem{}
	defer h.free()
	bindings := make([]C.GeasVowBinding, 0, len(vows))
	// Loop over every override
	for vow, val := range vows {
		// Encode the value
		cv, err := encode(h, val)
		if err != nil {
			return nil, fmt.Errorf("sign %s vow %s: %w", name, vow, err)
		}
		// The C vow name, kept until free
		cvow := (*C.char)(h.bytes(uintptr(len(vow) + 1)))
		copy(unsafe.Slice((*byte)(unsafe.Pointer(cvow)), len(vow)+1), append([]byte(vow), 0))
		bindings = append(bindings, C.GeasVowBinding{name: cvow, value: cv})
	}
	// The bindings pointer, nil when there are none
	var ptr *C.GeasVowBinding
	if len(bindings) > 0 {
		ptr = &bindings[0]
	}
	// Sign
	var c *C.GeasContract
	if err := check(C.geas_contract_sign(r.rt, cname, ptr, C.size_t(len(bindings)), 0, &c)); err != nil {
		return nil, fmt.Errorf("sign %s: %w", name, err)
	}
	return &Instance{c: c}, nil
}

/**
 * Shutdown
 * Joins the pool and frees every instance and module, nothing may call in after
 * @return void
 **/
func (r *Runtime) Shutdown() {
	C.geas_runtime_shutdown(r.rt)
}

// Instance is one signed contract
type Instance struct {
	// The instance
	c *C.GeasContract
}

/**
 * Fulfill
 * Runs a pledge by its bare name and waits for its value
 * @param pledge {string} - the pledge name
 * @param args {...Value} - the arguments
 * @return Value, error
 **/
func (i *Instance) Fulfill(pledge string, args ...Value) (Value, error) {
	// The C name
	cname := C.CString(pledge)
	defer C.free(unsafe.Pointer(cname))
	// The arguments live on the heap until fulfill copies them onto the instance
	h := &heapMem{}
	defer h.free()
	cargs := make([]C.GeasValue, 0, len(args))
	for n, a := range args {
		cv, err := encode(h, a)
		if err != nil {
			return nil, fmt.Errorf("fulfill %s arg %d: %w", pledge, n, err)
		}
		cargs = append(cargs, cv)
	}
	// The arguments pointer, nil when there are none
	var ptr *C.GeasValue
	if len(cargs) > 0 {
		ptr = &cargs[0]
	}
	// Fulfill and wait, the out slot lives on the heap so it is aligned the way C expects
	out := (*C.GeasValue)(h.bytes(valueSize))
	if err := check(C.geas_pledge_fulfill_sync(i.c, cname, ptr, C.size_t(len(cargs)), nil, 0, out)); err != nil {
		return nil, fmt.Errorf("fulfill %s: %w", pledge, err)
	}
	// Decode the value while the instance still owns it
	return decode(out), nil
}

/**
 * State
 * Returns the instance state
 * @return State
 **/
func (i *Instance) State() State {
	return State(C.geas_contract_state(i.c))
}

// PledgeError is one broken pledge and the Err payload it carried
type PledgeError struct {
	// The pledge name
	Pledge string
	// The Err payload
	Err Value
}

/**
 * Errors
 * Returns every broken pledge in declaration order
 * @return []PledgeError
 **/
func (i *Instance) Errors() []PledgeError {
	// The count
	n := int(C.geas_partial_nerrors(i.c))
	out := make([]PledgeError, 0, n)
	// Loop over every error
	for k := 0; k < n; k++ {
		var name *C.char
		var err *C.GeasValue
		if C.geas_partial_error(i.c, C.size_t(k), &name, &err) != C.GEAS_OK {
			continue
		}
		out = append(out, PledgeError{Pledge: C.GoString(name), Err: decode(err)})
	}
	return out
}

// ItemState is how an item of the partial surface reads
type ItemState int

// The item states
const (
	ItemPending ItemState = iota
	ItemFulfilled
	ItemBroken
)

/**
 * Items
 * Returns the names of every item reading as a state
 * @param k {ItemState} - the state
 * @return []string
 **/
func (i *Instance) Items(k ItemState) []string {
	// The count
	n := int(C.geas_partial_count(i.c, C.GeasItemState(k)))
	out := make([]string, 0, n)
	// Loop over every item
	for idx := 0; idx < n; idx++ {
		name := C.geas_partial_name(i.c, C.GeasItemState(k), C.size_t(idx))
		if name != nil {
			out = append(out, C.GoString(name))
		}
	}
	return out
}

/**
 * Vow
 * Reads a vow's signed value
 * @param name {string} - the vow name
 * @return Value, bool
 **/
func (i *Instance) Vow(name string) (Value, bool) {
	// The C name
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	// Read
	v := C.geas_vow_ref(i.c, cname)
	if v == nil {
		return nil, false
	}
	return decode(v), true
}

/**
 * Break
 * Frees everything the instance owns and latches it broken
 * @return error
 **/
func (i *Instance) Break() error {
	return check(C.geas_contract_break(i.c))
}
