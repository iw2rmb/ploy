// Package fixtures provides test data fixtures and sample files
package fixtures

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ApplicationTar represents a sample application tarball
type ApplicationTar struct {
	Name     string
	Language string
	Files    map[string]string // filename -> content
}

// GoApplicationFixtures returns common Go application fixtures
func GoApplicationFixtures() map[string]*ApplicationTar {
	return map[string]*ApplicationTar{
		"simple-go": {
			Name:     "simple-go-app",
			Language: "go",
			Files: map[string]string{
				"go.mod": `module simple-go-app

go 1.21

require github.com/gin-gonic/gin v1.9.1
`,
				"main.go": `package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r := gin.Default()
	
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Hello from simple-go-app!",
			"status":  "healthy",
		})
	})
	
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	log.Printf("Starting server on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
`,
				"Dockerfile": `FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE 8080
CMD ["./main"]
`,
				"README.md": `# Simple Go App

A simple Go application for testing Ploy deployment.

## Endpoints

- ` + "`" + `GET /` + "`" + ` - Returns a welcome message
- ` + "`" + `GET /health` + "`" + ` - Health check endpoint

## Environment Variables

- ` + "`" + `PORT` + "`" + ` - Server port (default: 8080)
`,
			},
		},
		"unikernel-go": {
			Name:     "unikernel-go-app",
			Language: "go",
			Files: map[string]string{
				"go.mod": `module unikernel-go-app

go 1.21
`,
				"main.go": `package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello from unikernel!")
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, ` + "`" + `{"status": "ok"}` + "`" + `)
	})

	log.Printf("Starting unikernel server on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
`,
				"kraft.yaml": `specification: v0.6
name: unikernel-go-app
unikraft:
  version: v0.14.0
  kconfig:
    - CONFIG_LIBPOSIX_MMAP=y
    - CONFIG_LIBUKSWRAND=y
    - CONFIG_LIBUKSWRAND_MWC=y
targets:
  - name: unikernel-go-app
    architecture: x86_64
    platform: linuxu
`,
				"README.md": `# Unikernel Go App

A Go application designed for unikernel deployment (Lane A/B).

## Features

- Minimal HTTP server
- Health check endpoint
- Unikraft configuration
`,
			},
		},
	}
}

// NodeApplicationFixtures returns common Node.js application fixtures
func NodeApplicationFixtures() map[string]*ApplicationTar {
	return map[string]*ApplicationTar{
		"simple-node": {
			Name:     "simple-node-app",
			Language: "javascript",
			Files: map[string]string{
				"package.json": `{
  "name": "simple-node-app",
  "version": "1.0.0",
  "description": "A simple Node.js application for testing",
  "main": "server.js",
  "scripts": {
    "start": "node server.js",
    "dev": "nodemon server.js",
    "test": "jest"
  },
  "dependencies": {
    "express": "^4.18.2",
    "morgan": "^1.10.0"
  },
  "devDependencies": {
    "nodemon": "^3.0.1",
    "jest": "^29.6.4"
  },
  "engines": {
    "node": ">=18.0.0"
  }
}`,
				"server.js": `const express = require('express');
const morgan = require('morgan');

const app = express();
const port = process.env.PORT || 3000;

// Middleware
app.use(morgan('combined'));
app.use(express.json());

// Routes
app.get('/', (req, res) => {
  res.json({
    message: 'Hello from simple-node-app!',
    status: 'healthy',
    timestamp: new Date().toISOString()
  });
});

app.get('/health', (req, res) => {
  res.json({
    status: 'ok',
    uptime: process.uptime()
  });
});

app.get('/info', (req, res) => {
  res.json({
    name: 'simple-node-app',
    version: '1.0.0',
    node: process.version,
    platform: process.platform
  });
});

// Error handling
app.use((err, req, res, next) => {
  console.error(err.stack);
  res.status(500).json({ error: 'Something went wrong!' });
});

app.listen(port, () => {
  console.log(` + "`" + `Server running on port ${port}` + "`" + `);
});
`,
				"Dockerfile": `FROM node:18-alpine

WORKDIR /usr/src/app

# Copy package files
COPY package*.json ./

# Install dependencies
RUN npm ci --only=production

# Copy source code
COPY . .

# Expose port
EXPOSE 3000

# Create non-root user
RUN addgroup -g 1001 -S nodejs
RUN adduser -S nextjs -u 1001

USER nextjs

CMD ["npm", "start"]
`,
				"README.md": `# Simple Node App

A simple Node.js/Express application for testing Ploy deployment.

## Endpoints

- ` + "`" + `GET /` + "`" + ` - Welcome message
- ` + "`" + `GET /health` + "`" + ` - Health check
- ` + "`" + `GET /info` + "`" + ` - Application info

## Environment Variables

- ` + "`" + `PORT` + "`" + ` - Server port (default: 3000)
`,
			},
		},
	}
}

// JavaApplicationFixtures returns common Java application fixtures
func JavaApplicationFixtures() map[string]*ApplicationTar {
	return map[string]*ApplicationTar{
		"spring-boot": {
			Name:     "spring-boot-app",
			Language: "java",
			Files: map[string]string{
				"build.gradle": `plugins {
    id 'java'
    id 'org.springframework.boot' version '3.1.2'
    id 'io.spring.dependency-management' version '1.1.2'
}

group = 'com.example'
version = '0.0.1-SNAPSHOT'

java {
    sourceCompatibility = '17'
}

repositories {
    mavenCentral()
}

dependencies {
    implementation 'org.springframework.boot:spring-boot-starter-web'
    implementation 'org.springframework.boot:spring-boot-starter-actuator'
    testImplementation 'org.springframework.boot:spring-boot-starter-test'
}

tasks.named('test') {
    useJUnitPlatform()
}
`,
				"src/main/java/com/example/Application.java": `package com.example;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

import java.time.Instant;
import java.util.Map;

@SpringBootApplication
@RestController
public class Application {

    public static void main(String[] args) {
        SpringApplication.run(Application.class, args);
    }

    @GetMapping("/")
    public Map<String, Object> home() {
        return Map.of(
            "message", "Hello from Spring Boot!",
            "status", "healthy",
            "timestamp", Instant.now()
        );
    }

    @GetMapping("/info")
    public Map<String, Object> info() {
        return Map.of(
            "name", "spring-boot-app",
            "version", "0.0.1-SNAPSHOT",
            "java", System.getProperty("java.version"),
            "spring", getClass().getPackage().getImplementationVersion()
        );
    }
}
`,
				"src/main/resources/application.yml": `server:
  port: ${PORT:8080}

management:
  endpoints:
    web:
      exposure:
        include: health,info,metrics
  endpoint:
    health:
      show-details: always

logging:
  level:
    com.example: INFO
    org.springframework: INFO
`,
				"Dockerfile": `FROM eclipse-temurin:17-jdk AS builder

WORKDIR /app
COPY . .
RUN ./gradlew build -x test

FROM eclipse-temurin:17-jre

RUN groupadd -r spring && useradd -r -g spring spring
USER spring

COPY --from=builder /app/build/libs/*.jar app.jar

EXPOSE 8080

ENTRYPOINT ["java", "-jar", "/app.jar"]
`,
				"README.md": `# Spring Boot App

A Spring Boot application for testing Ploy deployment.

## Endpoints

- ` + "`" + `GET /` + "`" + ` - Welcome message
- ` + "`" + `GET /info` + "`" + ` - Application info
- ` + "`" + `GET /actuator/health` + "`" + ` - Health check
- ` + "`" + `GET /actuator/info` + "`" + ` - Actuator info

## Build

` + "```" + `bash
./gradlew build
` + "```" + `
`,
			},
		},
	}
}

// WebAssemblyFixtures returns WASM application fixtures
func WebAssemblyFixtures() map[string]*ApplicationTar {
	return map[string]*ApplicationTar{
		"wasm-rust": {
			Name:     "wasm-rust-app",
			Language: "rust",
			Files: map[string]string{
				"Cargo.toml": `[package]
name = "wasm-rust-app"
version = "0.1.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[dependencies]
wasm-bindgen = "0.2"

[dependencies.web-sys]
version = "0.3"
features = [
  "console",
  "Response",
  "Request",
  "Headers",
]
`,
				"src/lib.rs": `use wasm_bindgen::prelude::*;

#[wasm_bindgen]
extern "C" {
    #[wasm_bindgen(js_namespace = console)]
    fn log(s: &str);
}

#[wasm_bindgen]
pub fn greet(name: &str) {
    log(&format!("Hello, {}!", name));
}

#[wasm_bindgen]
pub fn add(a: i32, b: i32) -> i32 {
    a + b
}

#[wasm_bindgen]
pub fn main() -> String {
    "Hello from WASM Rust app!".to_string()
}
`,
				"README.md": `# WASM Rust App

A Rust WebAssembly application for Lane G deployment.

## Build

\` + "`" + `\` + "`" + `\` + "`" + `bash
wasm-pack build --target web
\` + "`" + `\` + "`" + `\` + "`" + `
`,
			},
		},
	}
}

// CreateTarballFromFixture creates a tarball from an application fixture
func CreateTarballFromFixture(fixture *ApplicationTar) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add files to tarball
	for filename, content := range fixture.Files {
		header := &tar.Header{
			Name: filename,
			Mode: 0644,
			Size: int64(len(content)),
		}

		if err := tw.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("failed to write tar header: %w", err)
		}

		if _, err := tw.Write([]byte(content)); err != nil {
			return nil, fmt.Errorf("failed to write file content: %w", err)
		}
	}

	// Close tar and gzip writers
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar writer: %w", err)
	}

	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// ExtractTarballToDir extracts a tarball to a directory
func ExtractTarballToDir(tarballData []byte, destDir string) error {
	gr, err := gzip.NewReader(bytes.NewReader(tarballData))
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		target := filepath.Join(destDir, header.Name)

		// Create directory if needed
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		// Create file
		file, err := os.Create(target)
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}

		if _, err := io.Copy(file, tr); err != nil {
			file.Close()
			return fmt.Errorf("failed to copy file content: %w", err)
		}

		file.Close()

		// Set permissions
		if err := os.Chmod(target, os.FileMode(header.Mode)); err != nil {
			return fmt.Errorf("failed to set file permissions: %w", err)
		}
	}

	return nil
}

// NomadJobFixtures returns sample Nomad job specifications
func NomadJobFixtures() map[string]string {
	return map[string]string{
		"simple-web-app": `job "simple-web-app" {
  datacenters = ["local"]
  type        = "service"

  group "web" {
    count = 2

    network {
      port "http" {
        to = 8080
      }
    }

    service {
      name = "simple-web-app"
      port = "http"

      check {
        type     = "http"
        path     = "/health"
        interval = "10s"
        timeout  = "2s"
      }
    }

    task "app" {
      driver = "docker"

      config {
        image = "simple-web-app:latest"
        ports = ["http"]
      }

      resources {
        cpu    = 100
        memory = 128
      }
    }
  }
}`,
		"unikernel-app": `job "unikernel-app" {
  datacenters = ["local"]
  type        = "service"

  group "unikernel" {
    count = 1

    network {
      port "http" {
        to = 8080
      }
    }

    service {
      name = "unikernel-app"
      port = "http"
    }

    task "kernel" {
      driver = "raw_exec"

      config {
        command = "/opt/unikernel/app"
      }

      resources {
        cpu    = 50
        memory = 64
      }
    }
  }
}`,
	}
}

// ConsulServiceFixtures returns sample Consul service definitions
func ConsulServiceFixtures() map[string]interface{} {
	return map[string]interface{}{
		"web-service": map[string]interface{}{
			"ID":      "web-service-1",
			"Name":    "web-service",
			"Tags":    []string{"web", "api", "v1"},
			"Address": "10.0.1.100",
			"Port":    8080,
			"Meta": map[string]string{
				"version": "v1.0.0",
				"lane":    "E",
			},
			"Check": map[string]interface{}{
				"HTTP":     "http://10.0.1.100:8080/health",
				"Interval": "10s",
				"Timeout":  "3s",
			},
		},
	}
}

// DatabaseFixtures returns sample database records as JSON
func DatabaseFixtures() map[string]interface{} {
	return map[string]interface{}{
		"applications": []map[string]interface{}{
			{
				"id":       "app-001",
				"name":     "test-go-app",
				"language": "go",
				"lane":     "B",
				"status":   "deployed",
			},
			{
				"id":       "app-002",
				"name":     "test-node-app",
				"language": "javascript",
				"lane":     "E",
				"status":   "building",
			},
		},
		"deployments": []map[string]interface{}{
			{
				"id":           "deploy-001",
				"app_id":       "app-001",
				"version":      "v1.2.0",
				"artifact_url": "http://storage:8888/artifacts/test-go-app-v1.2.0.tar.gz",
				"status":       "healthy",
			},
		},
	}
}

// SaveFixtureToFile saves fixture data to a file
func SaveFixtureToFile(data interface{}, filename string) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal fixture data: %w", err)
	}

	if err := os.WriteFile(filename, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write fixture file: %w", err)
	}

	return nil
}

// LoadFixtureFromFile loads fixture data from a JSON file
func LoadFixtureFromFile(filename string, target interface{}) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read fixture file: %w", err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to unmarshal fixture data: %w", err)
	}

	return nil
}