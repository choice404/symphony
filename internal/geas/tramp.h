/* The trampolines that let the runtime call a Go function for a bound pledge.
 * The runtime binds a bare function pointer with no userdata, so every bound
 * pledge gets its own C function that carries a slot number into Go. */

#ifndef SYMPHONY_TRAMP_H
#define SYMPHONY_TRAMP_H

#include <geas/geas.h>

/* How many pledges a process can bind at once */
#define SYMPHONY_TRAMP_SLOTS 64

/* The trampoline for a slot, or NULL when the slot is out of range */
GeasPledgeFn symphony_tramp_at(int slot);

#endif
