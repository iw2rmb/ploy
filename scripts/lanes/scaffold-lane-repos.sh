#!/usr/bin/env bash
# Scaffold minimal hello apps into each lane repo so E2E can pass via container builds.
# Uses PAT and username from env. Creates language-appropriate HTTP servers with /healthz.

set -euo pipefail

USERNAME=${GITHUB_PLOY_DEV_USERNAME:-}
TOKEN=${GITHUB_PLOY_DEV_PAT:-}
if [[ -z "$USERNAME" || -z "$TOKEN" ]]; then
  echo "GITHUB_PLOY_DEV_USERNAME and GITHUB_PLOY_DEV_PAT are required" >&2
  exit 1
fi

ROOT=$(mktemp -d)
trap 'rm -rf "$ROOT"' EXIT

clone_repo() {
  local name=$1
  local url="https://${USERNAME}:${TOKEN}@github.com/${USERNAME}/${name}.git"
  git clone "$url" "$ROOT/$name" --depth 1
}

reset_repo() {
  local dir=$1
  find "$dir" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
}

commit_repo() {
  local dir=$1
  pushd "$dir" >/dev/null
  git config user.name "$USERNAME"
  git config user.email "${USERNAME}@users.noreply.github.com"
  git add .
  git commit -m "chore: scaffold minimal hello app with /healthz"
  git push origin HEAD:main || git push -u origin HEAD
  popd >/dev/null
}

write_file() { mkdir -p "$(dirname "$1")"; cat >"$1"; }

scaffold_go_docker() {
  local dir=$1
  write_file "$dir/main.go" << 'EOF'
package main

import (
  "fmt"
  "log"
  "net/http"
  "os"
)

func main() {
  mux := http.NewServeMux()
  mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte("ok"))
  })
  mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
    _, _ = w.Write([]byte("hello from ploy"))
  })
  port := os.Getenv("PORT")
  if port == "" { port = "8080" }
  addr := ":" + port
  log.Printf("listening on %s", addr)
  log.Fatal(http.ListenAndServe(addr, mux))
}
EOF
  write_file "$dir/go.mod" << 'EOF'
module example.com/hello

go 1.22
EOF
  write_file "$dir/Dockerfile" << 'EOF'
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app ./...

FROM gcr.io/distroless/static
ENV PORT=8080
EXPOSE 8080
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
EOF
}

scaffold_node_docker() {
  local dir=$1
  write_file "$dir/package.json" << 'EOF'
{
  "name": "ploy-node-hello",
  "version": "1.0.0",
  "main": "index.js",
  "license": "MIT"
}
EOF
  write_file "$dir/index.js" << 'EOF'
const http = require('http');
const port = process.env.PORT || 8080;

const server = http.createServer((req, res) => {
  if (req.url === '/healthz') {
    res.statusCode = 200;
    res.end('ok');
    return;
  }
  res.statusCode = 200;
  res.end('hello from ploy');
});

server.listen(port, () => {
  console.log(`listening on :${port}`);
});
EOF
  write_file "$dir/Dockerfile" << 'EOF'
FROM node:20-alpine
WORKDIR /app
COPY package.json index.js ./
ENV PORT=8080
EXPOSE 8080
CMD ["node", "index.js"]
EOF
}

scaffold_java_docker() {
  local dir=$1
  write_file "$dir/pom.xml" << 'EOF'
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>com.ploy</groupId>
  <artifactId>hello</artifactId>
  <version>1.0.0</</version>
  <properties>
    <maven.compiler.source>21</maven.compiler.source>
    <maven.compiler.target>21</maven.compiler.target>
  </properties>
  <dependencies>
    <dependency>
      <groupId>com.sparkjava</groupId>
      <artifactId>spark-core</artifactId>
      <version>2.9.4</version>
    </dependency>
  </dependencies>
  <build>
    <plugins>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-shade-plugin</artifactId>
        <version>3.5.0</version>
        <executions>
          <execution>
            <phase>package</phase>
            <goals><goal>shade</goal></goals>
            <configuration>
              <transformers>
                <transformer implementation="org.apache.maven.plugins.shade.resource.ManifestResourceTransformer">
                  <mainClass>com.ploy.hello.App</mainClass>
                </transformers>
              </transformers>
            </configuration>
          </execution>
        </executions>
      </plugin>
    </plugins>
  </build>
</project>
EOF
  write_file "$dir/src/main/java/com/ploy/hello/App.java" << 'EOF'
package com.ploy.hello;

import static spark.Spark.*;

public class App {
  public static void main(String[] args) {
    port(Integer.parseInt(System.getenv().getOrDefault("PORT", "8080")));
    get("/healthz", (req, res) -> "ok");
    get("/", (req, res) -> "hello from ploy");
  }
}
EOF
  write_file "$dir/Dockerfile" << 'EOF'
FROM maven:3.9.8-eclipse-temurin-21 AS build
WORKDIR /src
COPY pom.xml .
RUN mvn -q -e -DskipTests dependency:go-offline || true
COPY src ./src
RUN mvn -q -DskipTests package

FROM eclipse-temurin:21-jre
ENV PORT=8080
EXPOSE 8080
COPY --from=build /src/target/hello-1.0.0.jar /app/app.jar
ENTRYPOINT ["java","-jar","/app/app.jar"]
EOF
}

scaffold_python_docker() {
  local dir=$1
  write_file "$dir/requirements.txt" << 'EOF'
flask==3.0.3
gunicorn==22.0.0
EOF
  write_file "$dir/app.py" << 'EOF'
from flask import Flask
import os

app = Flask(__name__)

@app.get("/healthz")
def health():
    return "ok", 200

@app.get("/")
def root():
    return "hello from ploy", 200

if __name__ == "__main__":
    port = int(os.getenv("PORT", "8080"))
    app.run(host="0.0.0.0", port=port)
EOF
  write_file "$dir/Dockerfile" << 'EOF'
FROM python:3.11-slim
WORKDIR /app
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
ENV PORT=8080
EXPOSE 8080
CMD ["gunicorn", "-b", ":8080", "app:app"]
EOF
}

scaffold_dotnet_docker() {
  local dir=$1
  mkdir -p "$dir"
  pushd "$dir" >/dev/null
  dotnet new web -n HelloApp -o . >/dev/null 2>&1 || true
  # Replace Program.cs with minimal handlers
  write_file "$dir/Program.cs" << 'EOF'
var builder = WebApplication.CreateBuilder(args);
var app = builder.Build();
app.MapGet("/healthz", () => Results.Ok("ok"));
app.MapGet("/", () => "hello from ploy");
app.Run();
EOF
  popd >/dev/null
  write_file "$dir/Dockerfile" << 'EOF'
FROM mcr.microsoft.com/dotnet/sdk:8.0 AS build
WORKDIR /src
COPY . .
RUN dotnet publish -c Release -o /out

FROM mcr.microsoft.com/dotnet/aspnet:8.0
ENV ASPNETCORE_URLS=http://+:8080
EXPOSE 8080
COPY --from=build /out /app
WORKDIR /app
ENTRYPOINT ["dotnet", "HelloApp.dll"]
EOF
}

scaffold_rust_docker() {
  local dir=$1
  write_file "$dir/Cargo.toml" << 'EOF'
[package]
name = "hello"
version = "0.1.0"
edition = "2021"

[dependencies]
hyper = { version = "0.14", features = ["server", "http1"] }
tokio = { version = "1", features = ["macros", "rt-multi-thread"] }
EOF
  write_file "$dir/src/main.rs" << 'EOF'
use hyper::{Body, Request, Response, Server};
use hyper::service::{make_service_fn, service_fn};
use std::env;

async fn handle(req: Request<Body>) -> Result<Response<Body>, hyper::Error> {
    match req.uri().path() {
        "/healthz" => Ok(Response::new(Body::from("ok"))),
        _ => Ok(Response::new(Body::from("hello from ploy"))),
    }
}

#[tokio::main]
async fn main() {
    let port = env::var("PORT").unwrap_or_else(|_| "8080".into());
    let addr = format!("0.0.0.0:{}", port).parse().unwrap();
    let make_svc = make_service_fn(|_conn| async { Ok::<_, hyper::Error>(service_fn(handle)) });
    let server = Server::bind(&addr).serve(make_svc);
    println!("listening on {}", addr);
    if let Err(e) = server.await { eprintln!("server error: {}", e); }
}
EOF
  write_file "$dir/Dockerfile" << 'EOF'
FROM rust:1.79-slim AS build
WORKDIR /src
COPY Cargo.toml .
RUN mkdir -p src && echo "fn main(){}" > src/main.rs && cargo build --release || true
COPY . .
RUN cargo build --release

FROM debian:stable-slim
ENV PORT=8080
EXPOSE 8080
COPY --from=build /src/target/release/hello /app
ENTRYPOINT ["/app"]
EOF
}

main() {
  LANES=(A B C D E F G)
  NAMES=(
    ploy-lane-a-go
    ploy-lane-b-node
    ploy-lane-c-java
    ploy-lane-d-python
    ploy-lane-e-go
    ploy-lane-f-dotnet
    ploy-lane-g-rust
  )

  for i in "${!LANES[@]}"; do
    lane=${LANES[$i]}
    name=${NAMES[$i]}
    echo "==> Scaffold $lane: $name"
    clone_repo "$name"
    dir="$ROOT/$name"
    reset_repo "$dir"
    case "$lane" in
      A|E) scaffold_go_docker "$dir" ;;
      B) scaffold_node_docker "$dir" ;;
      C) scaffold_java_docker "$dir" ;;
      D) scaffold_python_docker "$dir" ;;
      F) scaffold_dotnet_docker "$dir" ;;
      G) scaffold_rust_docker "$dir" ;;
    esac
    write_file "$dir/README.md" << EOF
# $name

Minimal hello app for Ploy E2E. Exposes /healthz on PORT (default 8080).

- Language: lane $lane sample
- Default deployment path: container (Lane E) when not forced
- To force lane, set LANE env when running `ploy push` (may require lane-specific support on platform)
EOF
    commit_repo "$dir"
  done
}

main "$@"
