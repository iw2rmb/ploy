package orchestration

import ploynomad "github.com/iw2rmb/ploy/api/nomad"

// RenderData aliases the API's render data to keep call sites stable during migration
type RenderData = ploynomad.RenderData

// RenderTemplate delegates to the existing API implementation (temporary shim)
func RenderTemplate(lane string, data RenderData) (string, error) {
    return ploynomad.RenderTemplate(lane, data)
}

