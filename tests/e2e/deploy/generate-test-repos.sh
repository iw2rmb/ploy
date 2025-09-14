#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${GITHUB_PLOY_DEV_USERNAME:-}" || -z "${GITHUB_PLOY_DEV_PAT:-}" ]]; then
  echo "GITHUB_PLOY_DEV_USERNAME and GITHUB_PLOY_DEV_PAT are required" >&2
  exit 1
fi

USER="$GITHUB_PLOY_DEV_USERNAME"
AUTH_HEADER=( -H "Authorization: token ${GITHUB_PLOY_DEV_PAT}" -H 'Accept: application/vnd.github+json' )

api() { local m=$1; shift; local p=$1; shift; curl -fsS "https://api.github.com${p}" -X "$m" "${AUTH_HEADER[@]}" "$@"; }

create_repo() {
  local name=$1 desc=$2
  if api GET "/repos/${USER}/${name}" >/dev/null 2>&1; then echo "Repo exists: ${name}"; return; fi
  api POST "/user/repos" -d "$(jq -n --arg n "$name" --arg d "$desc" '{name:$n, description:$d, private:false, auto_init:true}')" >/dev/null
  echo "Created repo: ${name}"
}

commit_push() { local dir=$1; pushd "$dir" >/dev/null; git init -q; git checkout -b main >/dev/null 2>&1 || true; git add .; git -c user.email="bot@local" -c user.name="Ploy Bot" commit -qm init; git remote add origin "https://${USER}:${GITHUB_PLOY_DEV_PAT}@github.com/${USER}/$(basename "$dir").git" 2>/dev/null || true; git push -u origin main -f >/dev/null; popd >/dev/null; }

mk() { mkdir -p "$1"; }
write() { mk "$(dirname "$1")"; cat >"$1"; }

scaffold_node() { local d=$1 v=$2; write "$d/package.json" <<EOF
{ "name":"${d##*/}", "version":"1.0.0", "engines": {"node": ">${v}"}, "main":"index.js" }
EOF
write "$d/index.js" <<'EOF'
const http=require('http');const port=process.env.PORT||8080;http.createServer((req,res)=>{if(req.url==='/healthz'){res.statusCode=200;return res.end('ok');}res.end('hello');}).listen(port);
EOF
}

scaffold_go() { local d=$1 v=$2; write "$d/go.mod" <<EOF
module example.com/hello

go $v
EOF
write "$d/main.go" <<'EOF'
package main
import ("net/http";"os")
func main(){http.HandleFunc("/healthz",func(w http.ResponseWriter,_ *http.Request){w.WriteHeader(200);w.Write([]byte("ok"))});port:=os.Getenv("PORT");if port==""{port="8080"};http.ListenAndServe(":"+port,nil)}
EOF
}

scaffold_python() { local d=$1 v=$2; write "$d/runtime.txt" <<EOF
python-$v
EOF
write "$d/app.py" <<'EOF'
from http.server import BaseHTTPRequestHandler, HTTPServer
import os
class H(BaseHTTPRequestHandler):
  def do_GET(self):
    if self.path == '/healthz': self.send_response(200); self.end_headers(); self.wfile.write(b'ok'); return
    self.send_response(200); self.end_headers(); self.wfile.write(b'hello')
port = int(os.environ.get('PORT','8080'))
HTTPServer(('',port), H).serve_forever()
EOF
}

scaffold_java_gradle() { local d=$1 v=$2 main=$3; write "$d/settings.gradle.kts" <<EOF
rootProject.name = "${d##*/}"
EOF
write "$d/build.gradle.kts" <<EOF
plugins { application; id("com.google.cloud.tools.jib") version "3.4.0" }
repositories { mavenCentral() }
dependencies { implementation("io.undertow:undertow-core:2.3.12.Final") }
application { mainClass.set("$main") }
jib { from { image = "eclipse-temurin:${v}-jre" } to { image = System.getenv("JIB_TO_IMAGE") ?: "registry.dev.ployman.app/${d##*/}:dev" } container { ports=listOf("8080"); mainClass = "$main" } }
EOF
write "$d/src/main/java/${main//.//}/Main.java" <<'EOF'
package REPL;
import io.undertow.Undertow;import io.undertow.util.Headers;
public class Main { public static void main(String[] args){
  Undertow server = Undertow.builder().addHttpListener(Integer.parseInt(System.getenv().getOrDefault("PORT","8080")), "0.0.0.0")
    .setHandler(exchange -> { if ("/healthz".equals(exchange.getRequestPath())) { exchange.getResponseHeaders().put(Headers.CONTENT_TYPE, "text/plain"); exchange.getResponseSender().send("ok"); } else { exchange.getResponseSender().send("hello"); } }).build(); server.start(); }
}
EOF
perl -0777 -pe "s/REPL\./${main%.*}\./g; s/REPL/${main%.*}/g" -i "$d/src/main/java/${main//.//}/Main.java"
}

scaffold_repo() {
  local lane=$1 lang=$2 ver=$3; local name="ploy-lane-${lane,,}-${lang,,}-${ver}"
  create_repo "$name" "E2E ${lane}/${lang} ${ver}"
  local dir=$(mktemp -d)/$name; mk "$dir"
  case "$lang" in
    node) scaffold_node "$dir" "$ver" ;;
    go) scaffold_go "$dir" "$ver" ;;
    python) scaffold_python "$dir" "$ver" ;;
    java) scaffold_java_gradle "$dir" "$ver" "com.ploy.app.Main" ;;
    scala) scaffold_java_gradle "$dir" "$ver" "com.ploy.app.Main" ;;
  esac
  commit_push "$dir"
}

# Seed matrix (extend as needed)
scaffold_repo C scala 21
scaffold_repo C java 8
scaffold_repo E node 20
scaffold_repo E go 1.22
scaffold_repo E python 3.12
scaffold_repo E dotnet 8 || true
scaffold_repo G rust 1.79 || true

echo "Done."

