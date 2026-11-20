package api

import "github.com/iw2rmb/ploy/internal/domain/jvmversions"

// JVMVersionsResponse reports co-occurrence within observations, without compatibility claims.
type JVMVersionsResponse struct {
	Versions []jvmversions.Observation `json:"versions"`
}
