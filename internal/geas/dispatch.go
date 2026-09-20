//go:build geas

package geas

/*
#include <geas/geas.h>
*/
import "C"

import (
	"log"
	"unsafe"
)

/**
 * symphonyDispatch
 * The one Go entry every trampoline reaches, it finds the bound function by slot, decodes the frame, runs it, and encodes its value onto the instance
 * @param slot {C.int} - the trampoline slot
 * @param ctx {unsafe.Pointer} - the signed instance
 * @param args {*C.GeasValue} - the frame
 * @param nargs {C.size_t} - the frame length
 * @param out {*C.GeasValue} - where the value lands
 * @return C.GeasStatus
 **/
//export symphonyDispatch
func symphonyDispatch(slot C.int, ctx unsafe.Pointer, args *C.GeasValue, nargs C.size_t, out *C.GeasValue) (st C.GeasStatus) {
	// A panic in host code is a type error to the runtime, never a crash of the worker
	defer func() {
		if r := recover(); r != nil {
			log.Printf("geas: bound pledge panicked: %v", r)
			st = C.GEAS_ERR_TYPE
		}
	}()
	// Find the function
	slots.mu.RLock()
	fn := slots.fns[int(slot)]
	slots.mu.RUnlock()
	if fn == nil {
		return C.GEAS_ERR_UNBOUND
	}
	// Decode the frame
	in := make([]Value, 0, int(nargs))
	if nargs > 0 {
		// Decode in place, the frame stays in runtime memory
		frame := unsafe.Slice(args, int(nargs))
		for k := range frame {
			in = append(in, decode(&frame[k]))
		}
	}
	// Run it against the instance
	inst := &Instance{c: (*C.GeasContract)(ctx)}
	val, err := fn(inst, in)
	if err != nil {
		log.Printf("geas: bound pledge failed: %v", err)
		return C.GEAS_ERR_TYPE
	}
	// Encode the value onto the instance so it lives until break
	cv, err := encode(instanceMem{c: inst.c}, val)
	if err != nil {
		log.Printf("geas: bound pledge value: %v", err)
		return C.GEAS_ERR_TYPE
	}
	*out = cv
	return C.GEAS_OK
}
