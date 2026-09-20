//go:build geas && dusk

package mail

/*
#cgo LDFLAGS: ${SRCDIR}/../../target/dusk-out/libclassify.a -lgeasrt -lpthread -lm
#include <geas/geas.h>

// The dusk archive exports the pledge body in the frame shape every pledge takes
extern int32_t symphony_Mailbox_classify(void* ctx, void* args, uint64_t nargs, void* out);

// Hands the function out as a pointer, since cgo cannot take a C function's address from Go
static void* symphony_classify_ptr(void) { return (void*)symphony_Mailbox_classify; }
*/
import "C"

import (
	"github.com/choice404/symphony/internal/geas"
)

/**
 * BindDusk
 * Binds the classify pledge to the body linked in from the dusk archive
 * @param rt {*geas.Runtime} - the runtime with the mail module loaded
 * @return error
 **/
func BindDusk(rt *geas.Runtime) error {
	// Bind the archive's entry point straight to the pledge
	return rt.BindC(ContractName+".classify", C.symphony_classify_ptr())
}
