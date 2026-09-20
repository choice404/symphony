/* The glue between a dusk pledge body and the geas value ABI. Dusk reaches C
 * through scalars and raw pointers only, so these three functions read a
 * string argument out of a frame and write a Result back into the out slot,
 * and the dusk side never touches a GeasValue layout itself. Every byte they
 * allocate lives on the instance and dies at break. */

#include <geas/geas.h>
#include <string.h>

/* Copies argument i as a NUL terminated instance owned string, empty when the
 * slot is missing or not a string */
char* symphony_arg_string(void* ctx, const GeasValue* args, uint64_t nargs, uint64_t i) {
    GeasContract* c = (GeasContract*)ctx;
    /* A missing or non string slot reads as empty */
    if (i >= nargs || args[i].ty != GEAS_TY_STRING) {
        char* empty = (char*)geas_bytes(c, 1);
        if (empty) empty[0] = 0;
        return empty;
    }
    /* Copy the bytes and terminate them */
    uint64_t len = args[i].as.s.len;
    char* buf = (char*)geas_bytes(c, len + 1);
    if (!buf) return NULL;
    if (len) memcpy(buf, args[i].as.s.ptr, len);
    buf[len] = 0;
    return buf;
}

/* Writes Ok(s) into out */
int32_t symphony_out_ok_string(void* ctx, GeasValue* out, const char* s) {
    GeasContract* c = (GeasContract*)ctx;
    /* The boxed payload */
    GeasValue* box = geas_box(c);
    if (!box) return GEAS_ERR_OOM;
    *box = geas_string_copy(c, (const uint8_t*)s, strlen(s));
    /* The Result around it */
    out->ty = GEAS_TY_RESULT;
    out->tag = 0;
    out->as.box = box;
    return GEAS_OK;
}

/* Writes Err(variant tag carrying one string field) into out */
int32_t symphony_out_err_string(void* ctx, GeasValue* out, uint32_t tag, const char* detail) {
    GeasContract* c = (GeasContract*)ctx;
    /* The one field of the variant */
    GeasValue* fields = (GeasValue*)geas_bytes(c, sizeof(GeasValue));
    if (!fields) return GEAS_ERR_OOM;
    fields[0] = geas_string_copy(c, (const uint8_t*)detail, strlen(detail));
    /* The variant */
    GeasValue* box = geas_box(c);
    if (!box) return GEAS_ERR_OOM;
    box->ty = GEAS_TY_SUM;
    box->tag = tag;
    box->as.list.data = fields;
    box->as.list.len = 1;
    box->as.list.cap = 1;
    box->as.list.elem_ty = 0;
    /* The Result around it */
    out->ty = GEAS_TY_RESULT;
    out->tag = 1;
    out->as.box = box;
    return GEAS_OK;
}
