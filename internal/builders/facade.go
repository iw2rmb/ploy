package builders

import abuilders "github.com/iw2rmb/ploy/api/builders"

// JavaOSVRequest mirrors the API request subset
type JavaOSVRequest struct {
    App       string
    MainClass string
    SrcDir    string
    GitSHA    string
    OutDir    string
    EnvVars   map[string]string
}

func BuildUnikraft(app, lane, srcDir, sha, outDir string, envVars map[string]string) (string, error) {
    return abuilders.BuildUnikraft(app, lane, srcDir, sha, outDir, envVars)
}

func BuildOSVJava(req JavaOSVRequest) (string, error) {
    return abuilders.BuildOSVJava(abuilders.JavaOSVRequest{
        App:       req.App,
        MainClass: req.MainClass,
        SrcDir:    req.SrcDir,
        GitSHA:    req.GitSHA,
        OutDir:    req.OutDir,
        EnvVars:   req.EnvVars,
    })
}

func BuildJail(app, srcDir, sha, outDir string, envVars map[string]string) (string, error) {
    return abuilders.BuildJail(app, srcDir, sha, outDir, envVars)
}

func BuildOCI(app, srcDir, tag string, envVars map[string]string) (string, error) {
    return abuilders.BuildOCI(app, srcDir, tag, envVars)
}

func BuildVM(app, sha, outDir string, envVars map[string]string) (string, error) {
    return abuilders.BuildVM(app, sha, outDir, envVars)
}

