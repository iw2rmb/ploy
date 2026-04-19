import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.FileVisitResult;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.SimpleFileVisitor;
import java.nio.file.attribute.BasicFileAttributes;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.jar.JarEntry;
import java.util.jar.JarOutputStream;
import javax.tools.JavaCompiler;
import javax.tools.JavaFileObject;
import javax.tools.StandardJavaFileManager;
import javax.tools.ToolProvider;
import org.junit.jupiter.api.Assumptions;
import org.junit.jupiter.api.Test;

class DependencyUsageExtractorTest {
    @Test
    void extractVersionFromJarPathHandlesKnownLayouts() {
        class TestCase {
            final String name;
            final Path jarPath;
            final String expectedGroupId;
            final String expectedArtifactId;
            final String expectedVersion;

            TestCase(
                String name,
                Path jarPath,
                String expectedGroupId,
                String expectedArtifactId,
                String expectedVersion
            ) {
                this.name = name;
                this.jarPath = jarPath;
                this.expectedGroupId = expectedGroupId;
                this.expectedArtifactId = expectedArtifactId;
                this.expectedVersion = expectedVersion;
            }
        }

        List<TestCase> cases = Arrays.asList(
            new TestCase(
                "maven local repository path",
                Path.of(
                    "/tmp/.m2/repository/org/example/lib/1.2.3/lib-1.2.3.jar"
                ),
                "org.example",
                "lib",
                "1.2.3"
            ),
            new TestCase(
                "gradle modules cache path",
                Path.of(
                    "/tmp/.gradle/caches/modules-2/files-2.1/org.example/lib/4.5.6/abc/lib-4.5.6.jar"
                ),
                "org.example",
                "lib",
                "4.5.6"
            ),
            new TestCase(
                "unknown path",
                Path.of("/tmp/flat/lib.jar"),
                "unknown",
                "unknown",
                "unknown"
            )
        );

        for (TestCase testCase : cases) {
            DependencyUsageExtractor.DependencyCoordinates coordinates =
                DependencyUsageExtractor.extractDependencyFromJarPath(
                    testCase.jarPath
                );
            assertEquals(testCase.expectedGroupId, coordinates.getGroupId(), testCase.name);
            assertEquals(
                testCase.expectedArtifactId,
                coordinates.getArtifactId(),
                testCase.name
            );
            assertEquals(
                testCase.expectedVersion,
                DependencyUsageExtractor.extractVersionFromJarPath(
                    testCase.jarPath
                ),
                testCase.name
            );
        }
    }

    @Test
    void longestMatchingTargetPackagePrefersMostSpecificPrefix() {
        String actual = DependencyUsageExtractor.longestMatchingTargetPackage(
            "org.springframework.context.support",
            Arrays.asList("org.springframework", "org.springframework.context")
        );

        assertEquals("org.springframework.context", actual);
    }

    @Test
    void analyzeCollectsResolvedUsageFromMainSourcesOnly() throws Exception {
        JavaCompiler compiler = ToolProvider.getSystemJavaCompiler();
        Assumptions.assumeTrue(compiler != null, "JDK compiler is required");

        Path tempDir = Files.createTempDirectory("dep-usage-extractor-test");
        Path repoRoot = tempDir.resolve("repo");
        Path mainSourceDir = repoRoot.resolve("app/src/main/java/com/acme");
        Path testSourceDir = repoRoot.resolve("app/src/test/java/com/acme");
        Files.createDirectories(mainSourceDir);
        Files.createDirectories(testSourceDir);

        String mainSource =
            "package com.acme;\n" +
            "import org.example.lib.Api;\n" +
            "public class App {\n" +
            "  public void run() {\n" +
            "    Api api = new Api(\"value\");\n" +
            "    api.call(42);\n" +
            "    String s = api.field;\n" +
            "  }\n" +
            "}\n";
        Files.write(
            mainSourceDir.resolve("App.java"),
            mainSource.getBytes(StandardCharsets.UTF_8)
        );

        String testOnlySource =
            "package com.acme;\n" +
            "import org.example.lib.Api;\n" +
            "public class IgnoredTest {\n" +
            "  public void onlyInTests() {\n" +
            "    new Api(\"test\").testOnly();\n" +
            "  }\n" +
            "}\n";
        Files.write(
            testSourceDir.resolve("IgnoredTest.java"),
            testOnlySource.getBytes(StandardCharsets.UTF_8)
        );

        Path dependencyJar = createDependencyJar(
            tempDir.resolve("m2/repository/org/example/lib/1.2.3/lib-1.2.3.jar"),
            compiler
        );

        Path classpathFile = tempDir.resolve("java.classpath");
        Files.write(
            classpathFile,
            (dependencyJar.toAbsolutePath().toString() + "\n").getBytes(
                    StandardCharsets.UTF_8
                )
        );

        DependencyUsageExtractor extractor = new DependencyUsageExtractor();
        DependencyUsageExtractor.Result result = extractor.analyze(
            new DependencyUsageExtractor.Config(
                repoRoot,
                classpathFile,
                Collections.singletonList("org.example")
            )
        );

        assertEquals(1, result.getUsages().size());
        DependencyUsageExtractor.UsageGroup usage = result.getUsages().get(0);
        assertEquals("org.example:lib@1.2.3", usage.getGa());

        Set<String> symbols = new HashSet<String>(usage.getSymbols());
        assertTrue(symbols.contains("org.example.lib.Api"));
        assertTrue(symbols.contains("org.example.lib.Api#<init>(java.lang.String)"));
        assertTrue(symbols.contains("org.example.lib.Api#call(int)"));
        assertTrue(symbols.contains("org.example.lib.Api#field"));
        assertFalse(symbols.contains("org.example.lib.Api#testOnly()"));

        DependencyUsageExtractor.Result unfiltered = extractor.analyze(
            new DependencyUsageExtractor.Config(
                repoRoot,
                classpathFile,
                Collections.emptyList()
            )
        );
        assertTrue(!unfiltered.getUsages().isEmpty());
        boolean foundUnfilteredSymbol = false;
        for (DependencyUsageExtractor.UsageGroup usageGroup : unfiltered.getUsages()) {
            if (!"org.example:lib@1.2.3".equals(usageGroup.getGa())) {
                continue;
            }
            if (
                new HashSet<String>(usageGroup.getSymbols()).contains(
                        "org.example.lib.Api#call(int)"
                    )
            ) {
                foundUnfilteredSymbol = true;
                break;
            }
        }
        assertTrue(foundUnfilteredSymbol);
    }

    @Test
    void analyzeResolvesDependencyMethodsUsingClasspathDirectoryTypes()
        throws Exception {
        JavaCompiler compiler = ToolProvider.getSystemJavaCompiler();
        Assumptions.assumeTrue(compiler != null, "JDK compiler is required");

        Path tempDir = Files.createTempDirectory("dep-usage-extractor-dircp-test");
        Path repoRoot = tempDir.resolve("repo");
        Path mainSourceDir = repoRoot.resolve("src/main/java/com/acme");
        Path generatedSourceDir = repoRoot.resolve(
            "build/generated/sources/proto/main/java/com/acme/gen"
        );
        Path generatedClassesDir = repoRoot.resolve("build/classes/java/main");
        Files.createDirectories(mainSourceDir);
        Files.createDirectories(generatedSourceDir);
        Files.createDirectories(generatedClassesDir);

        String generatedTypeSource =
            "package com.acme.gen;\n" +
            "public class GenStub {\n" +
            "  public static GenStub create() { return new GenStub(); }\n" +
            "}\n";
        Path generatedTypeFile = generatedSourceDir.resolve("GenStub.java");
        Files.write(
            generatedTypeFile,
            generatedTypeSource.getBytes(StandardCharsets.UTF_8)
        );
        compileJavaSources(
            Collections.singletonList(generatedTypeFile),
            generatedClassesDir,
            Collections.emptyList(),
            compiler
        );

        String mainSource =
            "package com.acme;\n" +
            "import com.acme.gen.GenStub;\n" +
            "import org.example.lib.Api;\n" +
            "public class App {\n" +
            "  public void run() {\n" +
            "    Api.call(GenStub.create());\n" +
            "  }\n" +
            "}\n";
        Files.write(
            mainSourceDir.resolve("App.java"),
            mainSource.getBytes(StandardCharsets.UTF_8)
        );

        String dependencySource =
            "package org.example.lib;\n" +
            "import com.acme.gen.GenStub;\n" +
            "public class Api {\n" +
            "  public static void call(GenStub stub) {}\n" +
            "}\n";
        Path dependencyJar = createDependencyJar(
            tempDir.resolve("m2/repository/org/example/lib/1.2.3/lib-1.2.3.jar"),
            dependencySource,
            Collections.singletonList(generatedClassesDir),
            compiler
        );

        Path classpathFile = tempDir.resolve("java.classpath");
        Files.write(
            classpathFile,
            (
                generatedClassesDir.toAbsolutePath().toString() +
                "\n" +
                dependencyJar.toAbsolutePath().toString() +
                "\n"
            ).getBytes(StandardCharsets.UTF_8)
        );

        DependencyUsageExtractor extractor = new DependencyUsageExtractor();
        DependencyUsageExtractor.Result result = extractor.analyze(
            new DependencyUsageExtractor.Config(
                repoRoot,
                classpathFile,
                Collections.singletonList("org.example")
            )
        );

        assertEquals(1, result.getUsages().size());
        DependencyUsageExtractor.UsageGroup usage = result.getUsages().get(0);
        assertEquals("org.example:lib@1.2.3", usage.getGa());
        assertTrue(
            usage.getSymbols().contains(
                    "org.example.lib.Api#call(com.acme.gen.GenStub)"
                )
        );
    }

    private static Path createDependencyJar(Path jarPath, JavaCompiler compiler)
        throws IOException {
        String source =
            "package org.example.lib;\n" +
            "public class Api {\n" +
            "  public String field = \"x\";\n" +
            "  public Api(String value) {}\n" +
            "  public String call(int value) { return String.valueOf(value); }\n" +
            "  public void testOnly() {}\n" +
            "}\n";
        return createDependencyJar(
            jarPath,
            source,
            Collections.emptyList(),
            compiler
        );
    }

    private static Path createDependencyJar(
        Path jarPath,
        String source,
        List<Path> classpathEntries,
        JavaCompiler compiler
    ) throws IOException {
        Path sourceRoot = jarPath.getParent().resolve("src");
        Path sourceFile = sourceRoot.resolve("org/example/lib/Api.java");
        Files.createDirectories(sourceFile.getParent());
        Files.write(sourceFile, source.getBytes(StandardCharsets.UTF_8));

        Path classesDir = jarPath.getParent().resolve("classes");
        Files.createDirectories(classesDir);

        compileJavaSources(
            Collections.singletonList(sourceFile),
            classesDir,
            classpathEntries,
            compiler
        );

        List<Path> classFiles = collectClassFiles(classesDir);
        Files.createDirectories(jarPath.getParent());
        try (JarOutputStream out = new JarOutputStream(Files.newOutputStream(jarPath))) {
            for (Path classFile : classFiles) {
                String entryName = classesDir
                    .relativize(classFile)
                    .toString()
                    .replace('\\', '/');
                out.putNextEntry(new JarEntry(entryName));
                Files.copy(classFile, out);
                out.closeEntry();
            }
        }

        return jarPath;
    }

    private static void compileJavaSources(
        List<Path> sourceFiles,
        Path classesDir,
        List<Path> classpathEntries,
        JavaCompiler compiler
    ) throws IOException {
        try (
            StandardJavaFileManager fileManager = compiler.getStandardFileManager(
                null,
                null,
                StandardCharsets.UTF_8
            )
        ) {
            List<java.io.File> sourceFilesAsFiles = new ArrayList<java.io.File>();
            for (Path sourceFile : sourceFiles) {
                sourceFilesAsFiles.add(sourceFile.toFile());
            }
            Iterable<? extends JavaFileObject> compilationUnits =
                fileManager.getJavaFileObjectsFromFiles(
                sourceFilesAsFiles
            );
            List<String> options = new ArrayList<String>();
            options.add("-d");
            options.add(classesDir.toString());
            if (!classpathEntries.isEmpty()) {
                options.add("-classpath");
                options.add(
                    String.join(
                        System.getProperty("path.separator"),
                        classpathEntries
                            .stream()
                            .map(path -> path.toAbsolutePath().toString())
                            .toArray(String[]::new)
                    )
                );
            }

            Boolean ok = compiler
                .getTask(null, fileManager, null, options, null, compilationUnits)
                .call();
            assertTrue(Boolean.TRUE.equals(ok), "dependency compilation failed");
        }
    }

    private static List<Path> collectClassFiles(Path classesDir) throws IOException {
        List<Path> classFiles = new ArrayList<Path>();
        Files.walkFileTree(
            classesDir,
            new SimpleFileVisitor<Path>() {
                @Override
                public FileVisitResult visitFile(
                    Path file,
                    BasicFileAttributes attrs
                ) {
                    if (attrs.isRegularFile() && file.toString().endsWith(".class")) {
                        classFiles.add(file);
                    }
                    return FileVisitResult.CONTINUE;
                }
            }
        );
        Collections.sort(classFiles);
        return classFiles;
    }
}
