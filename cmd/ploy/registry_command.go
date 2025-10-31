package main

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "flag"
    "fmt"
    "io"
    "os"
    "strings"

    regcli "github.com/iw2rmb/ploy/internal/cli/registry"
)

func handleRegistry(args []string, stderr io.Writer) error {
    if len(args) == 0 {
        printRegistryUsage(stderr)
        return errors.New("registry subcommand required")
    }
    switch args[0] {
    case "push-blob":
        return handleRegistryPushBlob(args[1:], stderr)
    case "get-blob":
        return handleRegistryGetBlob(args[1:], stderr)
    case "rm-blob":
        return handleRegistryRemoveBlob(args[1:], stderr)
    case "put-manifest":
        return handleRegistryPutManifest(args[1:], stderr)
    case "get-manifest":
        return handleRegistryGetManifest(args[1:], stderr)
    case "rm-manifest":
        return handleRegistryRemoveManifest(args[1:], stderr)
    case "tags":
        return handleRegistryTags(args[1:], stderr)
    default:
        printRegistryUsage(stderr)
        return fmt.Errorf("unknown registry subcommand %q", args[0])
    }
}

func printRegistryUsage(w io.Writer) {
    _, _ = fmt.Fprintln(w, "Usage: ploy registry <push-blob|get-blob|rm-blob|put-manifest|get-manifest|rm-manifest|tags> [flags]")
}

// push-blob
func handleRegistryPushBlob(args []string, stderr io.Writer) error {
    fs := flag.NewFlagSet("registry push-blob", flag.ContinueOnError)
    fs.SetOutput(io.Discard)
    repo := fs.String("repo", "", "Repository, e.g. ploy/mods-openrewrite")
    media := fs.String("media-type", "application/octet-stream", "Blob media type")
    _ = fs.String("node-id", "", "(unused; HTTPS mode)")
    if err := fs.Parse(args); err != nil { printRegistryPushBlobUsage(stderr); return err }
    remaining := fs.Args()
    if strings.TrimSpace(*repo) == "" || len(remaining) == 0 {
        printRegistryPushBlobUsage(stderr)
        return errors.New("--repo and <path> are required")
    }
    path := remaining[0]
    if _, err := os.Stat(path); err != nil { return err }
    digest, err := fileSHA256(path)
    if err != nil { return err }
    ctx := context.Background()
    base, httpClient, err := resolveControlPlaneHTTP(ctx)
    if err != nil { return err }
    client := regcli.Client{BaseURL: base, HTTPClient: httpClient}
    // Prefer direct HTTPS upload using v2 alias.
    f, err := os.Open(path)
    if err != nil { return fmt.Errorf("open %s: %w", path, err) }
    defer f.Close()
    commit, err := client.UploadBlob(ctx, *repo, digest, *media, f)
    if err != nil { return err }
    _, _ = fmt.Fprintf(stderr, "Digest: %s\nCID: %s\n", strings.TrimSpace(commit.Digest), strings.TrimSpace(commit.CID))
    return nil
}

func printRegistryPushBlobUsage(w io.Writer) {
    _, _ = fmt.Fprintln(w, "Usage: ploy registry push-blob --repo <name> [--media-type <type>] [--node-id <node>] <path>")
}

// get-blob
func handleRegistryGetBlob(args []string, stderr io.Writer) error {
    fs := flag.NewFlagSet("registry get-blob", flag.ContinueOnError)
    fs.SetOutput(io.Discard)
    repo := fs.String("repo", "", "Repository name")
    digest := fs.String("digest", "", "sha256:<hex>")
    output := fs.String("output", "", "Output path")
    if err := fs.Parse(args); err != nil { printRegistryGetBlobUsage(stderr); return err }
    if strings.TrimSpace(*repo) == "" || strings.TrimSpace(*digest) == "" || strings.TrimSpace(*output) == "" {
        printRegistryGetBlobUsage(stderr)
        return errors.New("--repo, --digest and --output are required")
    }
    if err := os.MkdirAll(filepathDir(*output), 0o755); err != nil { return err }
    ctx := context.Background()
    base, httpClient, err := resolveControlPlaneHTTP(ctx)
    if err != nil { return err }
    client := regcli.Client{BaseURL: base, HTTPClient: httpClient}
    data, err := client.GetBlob(ctx, *repo, *digest)
    if err != nil { return err }
    if err := os.WriteFile(*output, data, 0o644); err != nil { return err }
    _, _ = fmt.Fprintf(stderr, "Wrote %d bytes to %s\n", len(data), *output)
    return nil
}

func printRegistryGetBlobUsage(w io.Writer) {
    _, _ = fmt.Fprintln(w, "Usage: ploy registry get-blob --repo <name> --digest <sha256:...> --output <path>")
}

// rm-blob
func handleRegistryRemoveBlob(args []string, stderr io.Writer) error {
    fs := flag.NewFlagSet("registry rm-blob", flag.ContinueOnError)
    fs.SetOutput(io.Discard)
    repo := fs.String("repo", "", "Repository name")
    digest := fs.String("digest", "", "sha256:<hex>")
    if err := fs.Parse(args); err != nil { printRegistryRemoveBlobUsage(stderr); return err }
    if strings.TrimSpace(*repo) == "" || strings.TrimSpace(*digest) == "" {
        printRegistryRemoveBlobUsage(stderr)
        return errors.New("--repo and --digest are required")
    }
    ctx := context.Background()
    base, httpClient, err := resolveControlPlaneHTTP(ctx)
    if err != nil { return err }
    client := regcli.Client{BaseURL: base, HTTPClient: httpClient}
    if err := client.DeleteBlob(ctx, *repo, *digest); err != nil { return err }
    _, _ = fmt.Fprintln(stderr, "Blob deleted")
    return nil
}

func printRegistryRemoveBlobUsage(w io.Writer) {
    _, _ = fmt.Fprintln(w, "Usage: ploy registry rm-blob --repo <name> --digest <sha256:...>")
}

// put-manifest
func handleRegistryPutManifest(args []string, stderr io.Writer) error {
    fs := flag.NewFlagSet("registry put-manifest", flag.ContinueOnError)
    fs.SetOutput(io.Discard)
    repo := fs.String("repo", "", "Repository name")
    ref := fs.String("reference", "", "Tag or digest reference")
    if err := fs.Parse(args); err != nil { printRegistryPutManifestUsage(stderr); return err }
    remaining := fs.Args()
    if strings.TrimSpace(*repo) == "" || strings.TrimSpace(*ref) == "" || len(remaining) == 0 {
        printRegistryPutManifestUsage(stderr)
        return errors.New("--repo, --reference and <manifest.json> are required")
    }
    manifest, err := os.ReadFile(remaining[0])
    if err != nil { return err }
    ctx := context.Background()
    base, httpClient, err := resolveControlPlaneHTTP(ctx)
    if err != nil { return err }
    client := regcli.Client{BaseURL: base, HTTPClient: httpClient}
    digest, err := client.PutManifest(ctx, *repo, *ref, manifest)
    if err != nil { return err }
    _, _ = fmt.Fprintf(stderr, "Manifest: %s\n", strings.TrimSpace(digest))
    return nil
}

func printRegistryPutManifestUsage(w io.Writer) {
    _, _ = fmt.Fprintln(w, "Usage: ploy registry put-manifest --repo <name> --reference <tag|digest> <manifest.json>")
}

// get-manifest
func handleRegistryGetManifest(args []string, stderr io.Writer) error {
    fs := flag.NewFlagSet("registry get-manifest", flag.ContinueOnError)
    fs.SetOutput(io.Discard)
    repo := fs.String("repo", "", "Repository name")
    ref := fs.String("reference", "", "Tag or digest reference")
    out := fs.String("output", "", "Output path")
    if err := fs.Parse(args); err != nil { printRegistryGetManifestUsage(stderr); return err }
    if strings.TrimSpace(*repo) == "" || strings.TrimSpace(*ref) == "" || strings.TrimSpace(*out) == "" {
        printRegistryGetManifestUsage(stderr)
        return errors.New("--repo, --reference and --output are required")
    }
    if err := os.MkdirAll(filepathDir(*out), 0o755); err != nil { return err }
    ctx := context.Background()
    base, httpClient, err := resolveControlPlaneHTTP(ctx)
    if err != nil { return err }
    client := regcli.Client{BaseURL: base, HTTPClient: httpClient}
    data, err := client.GetManifest(ctx, *repo, *ref)
    if err != nil { return err }
    if err := os.WriteFile(*out, data, 0o644); err != nil { return err }
    _, _ = fmt.Fprintf(stderr, "Wrote manifest to %s\n", *out)
    return nil
}

func printRegistryGetManifestUsage(w io.Writer) {
    _, _ = fmt.Fprintln(w, "Usage: ploy registry get-manifest --repo <name> --reference <ref> --output <path>")
}

// rm-manifest
func handleRegistryRemoveManifest(args []string, stderr io.Writer) error {
    fs := flag.NewFlagSet("registry rm-manifest", flag.ContinueOnError)
    fs.SetOutput(io.Discard)
    repo := fs.String("repo", "", "Repository name")
    ref := fs.String("reference", "", "Tag or digest reference")
    if err := fs.Parse(args); err != nil { printRegistryRemoveManifestUsage(stderr); return err }
    if strings.TrimSpace(*repo) == "" || strings.TrimSpace(*ref) == "" {
        printRegistryRemoveManifestUsage(stderr)
        return errors.New("--repo and --reference are required")
    }
    ctx := context.Background()
    base, httpClient, err := resolveControlPlaneHTTP(ctx)
    if err != nil { return err }
    client := regcli.Client{BaseURL: base, HTTPClient: httpClient}
    if err := client.DeleteManifest(ctx, *repo, *ref); err != nil { return err }
    _, _ = fmt.Fprintln(stderr, "Manifest/tag deleted")
    return nil
}

func printRegistryRemoveManifestUsage(w io.Writer) {
    _, _ = fmt.Fprintln(w, "Usage: ploy registry rm-manifest --repo <name> --reference <ref>")
}

// tags
func handleRegistryTags(args []string, stderr io.Writer) error {
    fs := flag.NewFlagSet("registry tags", flag.ContinueOnError)
    fs.SetOutput(io.Discard)
    repo := fs.String("repo", "", "Repository name")
    if err := fs.Parse(args); err != nil { printRegistryTagsUsage(stderr); return err }
    if strings.TrimSpace(*repo) == "" { printRegistryTagsUsage(stderr); return errors.New("--repo required") }
    ctx := context.Background()
    base, httpClient, err := resolveControlPlaneHTTP(ctx)
    if err != nil { return err }
    client := regcli.Client{BaseURL: base, HTTPClient: httpClient}
    tags, err := client.ListTags(ctx, *repo)
    if err != nil { return err }
    _, _ = fmt.Fprintf(stderr, "%s:\n", tags.Name)
    for _, t := range tags.Tags { _, _ = fmt.Fprintf(stderr, "  - %s\n", t) }
    return nil
}

func printRegistryTagsUsage(w io.Writer) {
    _, _ = fmt.Fprintln(w, "Usage: ploy registry tags --repo <name>")
}

// helpers
func fileSHA256(path string) (string, error) {
    f, err := os.Open(path)
    if err != nil { return "", err }
    defer f.Close()
    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil { return "", err }
    return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func filepathDir(p string) string {
    i := strings.LastIndexByte(p, '/')
    if i <= 0 { return "." }
    return p[:i]
}
