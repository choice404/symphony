/* One C function per slot, each handing its slot number to the Go dispatcher
 * that cgo exports as symphonyDispatch. */

#include "tramp.h"
#include "_cgo_export.h"

/* Defines the trampoline for one slot */
#define SYMPHONY_TRAMP(n)                                                        \
    static GeasStatus symphony_tramp_##n(void* ctx, const GeasValue* args,       \
                                         size_t nargs, GeasValue* out) {          \
        return symphonyDispatch(n, ctx, (GeasValue*)args, nargs, out);           \
    }

/* The sixty four trampolines */
SYMPHONY_TRAMP(0) SYMPHONY_TRAMP(1) SYMPHONY_TRAMP(2) SYMPHONY_TRAMP(3)
SYMPHONY_TRAMP(4) SYMPHONY_TRAMP(5) SYMPHONY_TRAMP(6) SYMPHONY_TRAMP(7)
SYMPHONY_TRAMP(8) SYMPHONY_TRAMP(9) SYMPHONY_TRAMP(10) SYMPHONY_TRAMP(11)
SYMPHONY_TRAMP(12) SYMPHONY_TRAMP(13) SYMPHONY_TRAMP(14) SYMPHONY_TRAMP(15)
SYMPHONY_TRAMP(16) SYMPHONY_TRAMP(17) SYMPHONY_TRAMP(18) SYMPHONY_TRAMP(19)
SYMPHONY_TRAMP(20) SYMPHONY_TRAMP(21) SYMPHONY_TRAMP(22) SYMPHONY_TRAMP(23)
SYMPHONY_TRAMP(24) SYMPHONY_TRAMP(25) SYMPHONY_TRAMP(26) SYMPHONY_TRAMP(27)
SYMPHONY_TRAMP(28) SYMPHONY_TRAMP(29) SYMPHONY_TRAMP(30) SYMPHONY_TRAMP(31)
SYMPHONY_TRAMP(32) SYMPHONY_TRAMP(33) SYMPHONY_TRAMP(34) SYMPHONY_TRAMP(35)
SYMPHONY_TRAMP(36) SYMPHONY_TRAMP(37) SYMPHONY_TRAMP(38) SYMPHONY_TRAMP(39)
SYMPHONY_TRAMP(40) SYMPHONY_TRAMP(41) SYMPHONY_TRAMP(42) SYMPHONY_TRAMP(43)
SYMPHONY_TRAMP(44) SYMPHONY_TRAMP(45) SYMPHONY_TRAMP(46) SYMPHONY_TRAMP(47)
SYMPHONY_TRAMP(48) SYMPHONY_TRAMP(49) SYMPHONY_TRAMP(50) SYMPHONY_TRAMP(51)
SYMPHONY_TRAMP(52) SYMPHONY_TRAMP(53) SYMPHONY_TRAMP(54) SYMPHONY_TRAMP(55)
SYMPHONY_TRAMP(56) SYMPHONY_TRAMP(57) SYMPHONY_TRAMP(58) SYMPHONY_TRAMP(59)
SYMPHONY_TRAMP(60) SYMPHONY_TRAMP(61) SYMPHONY_TRAMP(62) SYMPHONY_TRAMP(63)

/* The table the slots index */
static const GeasPledgeFn symphony_tramps[SYMPHONY_TRAMP_SLOTS] = {
    symphony_tramp_0,  symphony_tramp_1,  symphony_tramp_2,  symphony_tramp_3,
    symphony_tramp_4,  symphony_tramp_5,  symphony_tramp_6,  symphony_tramp_7,
    symphony_tramp_8,  symphony_tramp_9,  symphony_tramp_10, symphony_tramp_11,
    symphony_tramp_12, symphony_tramp_13, symphony_tramp_14, symphony_tramp_15,
    symphony_tramp_16, symphony_tramp_17, symphony_tramp_18, symphony_tramp_19,
    symphony_tramp_20, symphony_tramp_21, symphony_tramp_22, symphony_tramp_23,
    symphony_tramp_24, symphony_tramp_25, symphony_tramp_26, symphony_tramp_27,
    symphony_tramp_28, symphony_tramp_29, symphony_tramp_30, symphony_tramp_31,
    symphony_tramp_32, symphony_tramp_33, symphony_tramp_34, symphony_tramp_35,
    symphony_tramp_36, symphony_tramp_37, symphony_tramp_38, symphony_tramp_39,
    symphony_tramp_40, symphony_tramp_41, symphony_tramp_42, symphony_tramp_43,
    symphony_tramp_44, symphony_tramp_45, symphony_tramp_46, symphony_tramp_47,
    symphony_tramp_48, symphony_tramp_49, symphony_tramp_50, symphony_tramp_51,
    symphony_tramp_52, symphony_tramp_53, symphony_tramp_54, symphony_tramp_55,
    symphony_tramp_56, symphony_tramp_57, symphony_tramp_58, symphony_tramp_59,
    symphony_tramp_60, symphony_tramp_61, symphony_tramp_62, symphony_tramp_63,
};

/* Returns the trampoline for a slot */
GeasPledgeFn symphony_tramp_at(int slot) {
    /* Refuse a slot outside the table */
    if (slot < 0 || slot >= SYMPHONY_TRAMP_SLOTS) {
        return NULL;
    }
    /* Return the function */
    return symphony_tramps[slot];
}
