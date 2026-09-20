//go:build geas && !dusk

package mail

import (
	"errors"

	"github.com/choice404/symphony/internal/geas"
)

// errNoDusk says this build carries no dusk body
var errNoDusk = errors.New("classify not linked in, build with the dusk tag")

/**
 * BindDusk
 * Reports that this build has no dusk body to bind
 * @param rt {*geas.Runtime} - the runtime, unused
 * @return error
 **/
func BindDusk(rt *geas.Runtime) error {
	return errNoDusk
}
