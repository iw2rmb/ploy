package orchestration

import platformnomad "github.com/iw2rmb/ploy/platform/nomad"

// getEmbeddedTemplate returns embedded Nomad templates shared with platform/nomad.
func getEmbeddedTemplate(path string) []byte {
	return platformnomad.GetEmbeddedTemplate(path)
}
