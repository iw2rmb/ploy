package recipes

import (
    "fmt"
    "io"
    "net/http"
    "strings"
)

// MavenFetcher downloads artifacts from a Maven-style repository.
// URL layout:
//   {BaseURL}/{groupId with slashes}/{artifactId}/{version}/{artifactId}-{version}.jar
// Example:
//   BaseURL: https://repo1.maven.org/maven2
//   GroupID: org.openrewrite
//   pack: rewrite-java, version: 8.1.0
//   => https://repo1.maven.org/maven2/org/openrewrite/rewrite-java/8.1.0/rewrite-java-8.1.0.jar
type MavenFetcher struct {
    BaseURL string
    GroupID string
    Client  *http.Client
}

func (f MavenFetcher) Fetch(pack, version string) ([]byte, error) {
    gpath := strings.ReplaceAll(f.GroupID, ".", "/")
    url := fmt.Sprintf("%s/%s/%s/%s/%s-%s.jar", strings.TrimRight(f.BaseURL, "/"), gpath, pack, version, pack, version)
    client := f.Client
    if client == nil {
        client = http.DefaultClient
    }
    resp, err := client.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return nil, fmt.Errorf("fetch %s: status %d", url, resp.StatusCode)
    }
    return io.ReadAll(resp.Body)
}

