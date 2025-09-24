package mods

import "context"

// noopArtifactUploader is a test helper that satisfies ArtifactUploader while discarding uploads.
type noopArtifactUploader struct{}

func (noopArtifactUploader) UploadFile(context.Context, string, string, string, string) error {
	return nil
}

func (noopArtifactUploader) UploadJSON(context.Context, string, string, []byte) error { return nil }
