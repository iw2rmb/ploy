package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func scaffold(lang, name string) error {
	appDir := filepath.Join("apps", name)
	if err := os.MkdirAll(appDir, 0755); err != nil { return err }
	switch lang {
	case "go":
		return os.WriteFile(filepath.Join(appDir, "main.go"), []byte("package main\nimport (\n\t\"net/http\"\n\t\"log\"\n)\nfunc main(){http.HandleFunc(\"/healthz\", func(w http.ResponseWriter,_ *http.Request){w.Write([]byte(\"ok\"))}); log.Fatal(http.ListenAndServe(\":8080\", nil))}"), 0644)
	case "node":
		_ = os.WriteFile(filepath.Join(appDir, "package.json"), []byte("{\"name\":\""+name+"\",\"version\":\"0.1.0\",\"main\":\"server.js\",\"dependencies\":{\"express\":\"^4\"}}"), 0644)
		return os.WriteFile(filepath.Join(appDir, "server.js"), []byte("const e=require('express')(); e.get('/healthz',(a,b)=>b.send('ok')); e.listen(8080);"), 0644)
	default:
		return fmt.Errorf("unsupported lang: %s", lang)
	}
}
