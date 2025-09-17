package build

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

var registryHTTPClient = &http.Client{Timeout: 10 * time.Second}

// verifyResult represents the outcome of an OCI manifest existence check
type verifyResult struct {
	OK      bool
	Status  int
	Digest  string
	Message string
}

// verifyOCIPush performs a lightweight registry check to verify that the
// pushed reference exists. It issues a HEAD request to the registry v2 API
// and reads Docker-Content-Digest when available. Best-effort only.
func verifyOCIPush(tag string) verifyResult {
	// Expect tags like: host/repo[:tag]|[@digest]
	slash := strings.Index(tag, "/")
	if slash <= 0 || slash >= len(tag)-1 {
		return verifyResult{OK: false, Status: 0, Message: "unverifiable tag format"}
	}
	host := tag[:slash]
	remainder := tag[slash+1:]
	ref := "latest"
	name := remainder
	if at := strings.Index(remainder, "@"); at != -1 {
		name = remainder[:at]
		ref = remainder[at+1:]
	} else if colon := strings.LastIndex(remainder, ":"); colon != -1 {
		name = remainder[:colon]
		ref = remainder[colon+1:]
	}

	// Build v2 manifest URL
	u := url.URL{Scheme: "https", Host: host, Path: "/v2/" + name + "/manifests/" + ref}
	req, _ := http.NewRequest("HEAD", u.String(), nil)
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))
	client := registryHTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return verifyResult{OK: false, Status: 0, Message: "registry check failed: " + err.Error()}
	}
	// Some registries may not support HEAD. Fall back to GET on 405.
	if resp.StatusCode == http.StatusMethodNotAllowed {
		req.Method = "GET"
		_ = resp.Body.Close()
		resp, err = client.Do(req)
		if err != nil {
			return verifyResult{OK: false, Status: 0, Message: "registry GET failed: " + err.Error()}
		}
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	vr := verifyResult{Status: resp.StatusCode}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		vr.OK = true
		vr.Digest = resp.Header.Get("Docker-Content-Digest")
		if vr.Digest == "" {
			vr.Message = "manifest present (digest unavailable)"
		} else {
			vr.Message = "manifest present"
		}
		return vr
	}
	// Common outcomes
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		vr.Message = "unauthorized: ensure docker login on build host and pull credentials on Nomad nodes"
	case http.StatusNotFound:
		vr.Message = "manifest unknown: image tag not found in registry"
	default:
		vr.Message = "registry responded with status"
	}
	return vr
}
