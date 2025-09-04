package supply

import asupply "github.com/iw2rmb/ploy/api/supply"

func GenerateSBOM(target, lane, app, sha string) error { return asupply.GenerateSBOM(target, lane, app, sha) }

type SBOMOptions = asupply.SBOMGenerationOptions
type SBOMGenerator = asupply.SBOMGenerator

func DefaultSBOMOptions() SBOMOptions { return asupply.DefaultSBOMOptions() }
func NewSBOMGenerator() *SBOMGenerator { return asupply.NewSBOMGenerator() }

func SignArtifact(path string) error { return asupply.SignArtifact(path) }
func SignDockerImage(image string) error { return asupply.SignDockerImage(image) }
func VerifySignature(path, sig string) error { return asupply.VerifySignature(path, sig) }
