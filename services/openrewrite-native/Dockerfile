# Multi-stage build for GraalVM native OpenRewrite
FROM ghcr.io/graalvm/graalvm-ce:ol9-java17-22.3.3 AS builder

# Install native-image and Maven
RUN gu install native-image && \
    microdnf install -y maven git && \
    microdnf clean all

WORKDIR /build

# Create a simple OpenRewrite runner with embedded recipes
RUN cat > pom.xml << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>io.ploy</groupId>
    <artifactId>openrewrite-native</artifactId>
    <version>1.0.0</version>
    
    <properties>
        <maven.compiler.source>17</maven.compiler.source>
        <maven.compiler.target>17</maven.compiler.target>
        <openrewrite.version>8.21.0</openrewrite.version>
    </properties>
    
    <dependencies>
        <dependency>
            <groupId>org.openrewrite</groupId>
            <artifactId>rewrite-java</artifactId>
            <version>${openrewrite.version}</version>
        </dependency>
        <dependency>
            <groupId>org.openrewrite.recipe</groupId>
            <artifactId>rewrite-migrate-java</artifactId>
            <version>2.5.0</version>
        </dependency>
        <dependency>
            <groupId>org.openrewrite.recipe</groupId>
            <artifactId>rewrite-spring</artifactId>
            <version>5.7.0</version>
        </dependency>
        <dependency>
            <groupId>com.fasterxml.jackson.core</groupId>
            <artifactId>jackson-databind</artifactId>
            <version>2.15.2</version>
        </dependency>
    </dependencies>
    
    <build>
        <plugins>
            <plugin>
                <groupId>org.graalvm.buildtools</groupId>
                <artifactId>native-maven-plugin</artifactId>
                <version>0.9.24</version>
                <configuration>
                    <imageName>openrewrite-native</imageName>
                    <mainClass>io.ploy.OpenRewriteRunner</mainClass>
                    <buildArgs>
                        <arg>--no-fallback</arg>
                        <arg>--initialize-at-build-time</arg>
                        <arg>-H:+ReportExceptionStackTraces</arg>
                        <arg>--enable-url-protocols=https</arg>
                    </buildArgs>
                </configuration>
            </plugin>
        </plugins>
    </build>
</project>
EOF

# Create the OpenRewrite runner
RUN mkdir -p src/main/java/io/ploy && \
    cat > src/main/java/io/ploy/OpenRewriteRunner.java << 'EOF'
package io.ploy;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.openrewrite.*;
import org.openrewrite.config.Environment;
import org.openrewrite.java.JavaParser;
import org.openrewrite.java.migrate.UpgradeToJava17;
import org.openrewrite.style.NamedStyles;

import java.io.*;
import java.nio.file.*;
import java.nio.file.attribute.BasicFileAttributes;
import java.util.*;
import java.util.stream.Collectors;

public class OpenRewriteRunner {
    private static final ObjectMapper mapper = new ObjectMapper();
    
    public static void main(String[] args) throws Exception {
        if (args.length < 3) {
            System.err.println("Usage: openrewrite-native <input-tar> <output-tar> <recipe>");
            System.exit(1);
        }
        
        String inputTar = args[0];
        String outputTar = args[1];
        String recipeName = args[2];
        
        // Extract input tar
        Path workDir = Files.createTempDirectory("openrewrite");
        extractTar(inputTar, workDir);
        
        // Run transformation
        Map<String, Object> result = runTransformation(workDir, recipeName);
        
        // Create output tar
        createTar(workDir, outputTar);
        
        // Write result JSON
        String resultPath = outputTar.replace(".tar", ".json");
        mapper.writeValue(new File(resultPath), result);
        
        // Cleanup
        deleteDirectory(workDir);
    }
    
    private static void extractTar(String tarPath, Path destDir) throws Exception {
        ProcessBuilder pb = new ProcessBuilder("tar", "-xf", tarPath, "-C", destDir.toString());
        Process p = pb.start();
        if (p.waitFor() != 0) {
            throw new RuntimeException("Failed to extract tar");
        }
    }
    
    private static void createTar(Path sourceDir, String tarPath) throws Exception {
        ProcessBuilder pb = new ProcessBuilder("tar", "-cf", tarPath, "-C", sourceDir.toString(), ".");
        Process p = pb.start();
        if (p.waitFor() != 0) {
            throw new RuntimeException("Failed to create tar");
        }
    }
    
    private static Map<String, Object> runTransformation(Path projectDir, String recipeName) {
        Map<String, Object> result = new HashMap<>();
        result.put("recipe", recipeName);
        result.put("start_time", System.currentTimeMillis());
        
        try {
            // Create parser
            JavaParser javaParser = JavaParser.fromJavaVersion()
                .classpath(Collections.emptyList())
                .build();
            
            // Find Java files
            List<Path> javaFiles = Files.walk(projectDir)
                .filter(p -> p.toString().endsWith(".java"))
                .collect(Collectors.toList());
            
            // Parse files
            List<SourceFile> sources = javaParser.parse(javaFiles, projectDir, new InMemoryExecutionContext());
            
            // Create recipe based on name
            Recipe recipe = createRecipe(recipeName);
            
            // Run recipe
            List<Result> results = recipe.run(sources, new InMemoryExecutionContext());
            
            // Apply changes
            int changedFiles = 0;
            for (Result res : results) {
                if (res.diff() != null && !res.diff().isEmpty()) {
                    Path filePath = projectDir.resolve(res.getSourcePath());
                    Files.write(filePath, res.getAfter().printAll().getBytes());
                    changedFiles++;
                }
            }
            
            result.put("success", true);
            result.put("files_changed", changedFiles);
            result.put("total_files", javaFiles.size());
            
        } catch (Exception e) {
            result.put("success", false);
            result.put("error", e.getMessage());
        }
        
        result.put("end_time", System.currentTimeMillis());
        return result;
    }
    
    private static Recipe createRecipe(String recipeName) {
        switch (recipeName) {
            case "java11to17":
                return new UpgradeToJava17();
            case "java8to11":
                // For now, return noop for Java 8 to 11
                return Recipe.noop();
            default:
                // Return a no-op recipe for unknown recipes
                return Recipe.noop();
        }
    }
    
    private static void deleteDirectory(Path dir) throws IOException {
        Files.walkFileTree(dir, new SimpleFileVisitor<Path>() {
            @Override
            public FileVisitResult visitFile(Path file, BasicFileAttributes attrs) throws IOException {
                Files.delete(file);
                return FileVisitResult.CONTINUE;
            }
            
            @Override
            public FileVisitResult postVisitDirectory(Path dir, IOException exc) throws IOException {
                Files.delete(dir);
                return FileVisitResult.CONTINUE;
            }
        });
    }
}
EOF

# Build native image
RUN mvn clean compile && \
    mvn -Pnative -DskipTests package

# Final minimal image
FROM gcr.io/distroless/base-debian12

COPY --from=builder /build/target/openrewrite-native /openrewrite

ENTRYPOINT ["/openrewrite"]