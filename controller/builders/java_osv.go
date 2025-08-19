package builders

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type JavaOSVRequest struct {
	App       string
	MainClass string
	SrcDir    string // source directory
	JibTar    string // optional
	GitSHA    string
	OutDir    string
	EnvVars   map[string]string // environment variables
}

func BuildOSVJava(req JavaOSVRequest) (string, error) {
	if req.SrcDir == "" && req.JibTar == "" {
		return "", errors.New("either SrcDir or JibTar must be provided")
	}
	jibTar := req.JibTar
	if jibTar == "" {
		var err error
		jibTar, err = runJibBuildTar(req.SrcDir, req.EnvVars)
		if err != nil { return "", err }
	}
	out := filepath.Join(req.OutDir, fmt.Sprintf("%s-%s.qcow2", req.App, short(req.GitSHA)))
	args := []string{ "--tar", jibTar, "--main", req.MainClass, "--app", req.App, "--sha", req.GitSHA, "--out", out }
	cmd := exec.Command("./scripts/build/osv/java/build_osv_java_with_capstan.sh", args...)
	cmd.Stdout = os.Stdout; cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil { return "", err }
	return out, nil
}

func runJibBuildTar(src string, envVars map[string]string) (string, error) {
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	
	if exists(filepath.Join(src,"gradlew")) && (exists(filepath.Join(src,"build.gradle")) || exists(filepath.Join(src,"build.gradle.kts"))) {
		cmd := exec.Command("./gradlew","jibBuildTar"); cmd.Dir = src; cmd.Env = env; cmd.Stdout=os.Stdout; cmd.Stderr=os.Stderr; if err:=cmd.Run(); err==nil { p:=filepath.Join(src,"build","jib-image.tar"); if exists(p){return p,nil} }
	}
	if exists(filepath.Join(src,"mvnw")) && exists(filepath.Join(src,"pom.xml")) {
		cmd := exec.Command("./mvnw","-B","com.google.cloud.tools:jib-maven-plugin:buildTar"); cmd.Dir = src; cmd.Env = env; cmd.Stdout=os.Stdout; cmd.Stderr=os.Stderr; if err:=cmd.Run(); err==nil { p:=filepath.Join(src,"target","jib-image.tar"); if exists(p){return p,nil} }
	}
	return "", errors.New("failed to produce Jib tar")
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }
func short(s string) string { if len(s)>12 { return s[:12] }; return s }
