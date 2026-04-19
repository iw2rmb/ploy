import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
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
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.jar.JarEntry;
import java.util.jar.JarOutputStream;
import javax.tools.JavaCompiler;
import javax.tools.JavaFileObject;
import javax.tools.StandardJavaFileManager;
import javax.tools.ToolProvider;
import org.junit.jupiter.api.Assumptions;
import org.junit.jupiter.api.Test;

class DependencyDeprecatedUsageReportComposerTest {
    @Test
    void parseSymbolRefSupportsTypeFieldMethodAndConstructor() {
        class Case {
            final String symbol;
            final DependencyDeprecatedUsageReportComposer.SymbolKind kind;
            final String owner;
            final String member;
            final List<String> params;

            Case(
                String symbol,
                DependencyDeprecatedUsageReportComposer.SymbolKind kind,
                String owner,
                String member,
                List<String> params
            ) {
                this.symbol = symbol;
                this.kind = kind;
                this.owner = owner;
                this.member = member;
                this.params = params;
            }
        }

        List<Case> cases = Arrays.asList(
            new Case(
                "org.example.Type",
                DependencyDeprecatedUsageReportComposer.SymbolKind.TYPE,
                "org.example.Type",
                "",
                Collections.emptyList()
            ),
            new Case(
                "org.example.Type#FIELD",
                DependencyDeprecatedUsageReportComposer.SymbolKind.FIELD,
                "org.example.Type",
                "FIELD",
                Collections.emptyList()
            ),
            new Case(
                "org.example.Type#call(java.lang.String,int)",
                DependencyDeprecatedUsageReportComposer.SymbolKind.METHOD,
                "org.example.Type",
                "call",
                Arrays.asList("java.lang.String", "int")
            ),
            new Case(
                "org.example.Type#<init>(java.lang.String)",
                DependencyDeprecatedUsageReportComposer.SymbolKind.CONSTRUCTOR,
                "org.example.Type",
                "<init>",
                Collections.singletonList("java.lang.String")
            )
        );

        for (Case testCase : cases) {
            DependencyDeprecatedUsageReportComposer.SymbolRef parsed =
                DependencyDeprecatedUsageReportComposer.SymbolRef.parse(
                    testCase.symbol
                );
            assertNotNull(parsed);
            assertEquals(testCase.kind, parsed.getKind());
            assertEquals(testCase.owner, parsed.getOwnerClass());
            assertEquals(testCase.member, parsed.getMemberName());
            assertEquals(testCase.params, parsed.getParameterTypes());
        }
    }

    @Test
    void composeBuildsDeprecationCatalogWithCurrentVersionJavadocNotes()
        throws Exception {
        JavaCompiler compiler = ToolProvider.getSystemJavaCompiler();
        Assumptions.assumeTrue(compiler != null, "JDK compiler is required");

        Path tempDir = Files.createTempDirectory("deprecated-usage-composer-v2-");
        Path artifactRoot = tempDir.resolve("m2/repository/org/example/lib");

        createVersionArtifacts(
            artifactRoot,
            "0.9",
            sourceV09(),
            sourceV09(),
            compiler
        );
        Path v10Jar = createVersionArtifacts(
            artifactRoot,
            "1.0",
            sourceV10Binary(),
            sourceV10Sources(),
            compiler
        );
        createVersionArtifacts(
            artifactRoot,
            "1.1",
            sourceV11Binary(),
            sourceV11Sources(),
            compiler
        );

        Path classpathFile = tempDir.resolve("java.classpath");
        Files.write(
            classpathFile,
            (v10Jar.toAbsolutePath().normalize().toString() + "\n").getBytes(
                    StandardCharsets.UTF_8
                )
        );

        Path usageReport = tempDir.resolve("dependency-usage.json");
        writeUsageReport(
            usageReport,
            Arrays.asList(
                "org.example.lib.Api#old()",
                "org.example.lib.Api#bytecodeOnly()",
                "org.example.lib.Api#genericBytecodeOnly(T)",
                "org.example.lib.Api#genericWithDoc(T)",
                "org.example.lib.Api#active()"
            )
        );

        DependencyDeprecatedUsageReportComposer composer =
            new DependencyDeprecatedUsageReportComposer();
        List<DependencyDeprecatedUsageReportComposer.ReportGroup> report =
            composer.compose(
                new DependencyDeprecatedUsageReportComposer.Config(
                    usageReport,
                    classpathFile
                )
            );

        assertEquals(1, report.size());
        DependencyDeprecatedUsageReportComposer.ReportGroup group = report.get(0);
        assertEquals("org.example:lib@1.0", group.getGa());

        Map<String, DependencyDeprecatedUsageReportComposer.ReportSymbol> symbolsByName =
            bySymbol(group.getSymbols());
        assertEquals(4, symbolsByName.size());
        assertFalse(symbolsByName.containsKey("org.example.lib.Api#onlyInNewer()"));
        assertFalse(symbolsByName.containsKey("org.example.lib.Api#oldNoDoc()"));

        assertSymbol(
            symbolsByName.get("org.example.lib.Api#old()"),
            "use betterApi()"
        );
        assertSymbol(
            symbolsByName.get("org.example.lib.Api#bytecodeOnly()"),
            null
        );
        assertSymbol(
            symbolsByName.get("org.example.lib.Api#genericBytecodeOnly(T)"),
            null
        );
        assertSymbol(
            symbolsByName.get("org.example.lib.Api#genericWithDoc(T)"),
            "use genericReplacement(T)"
        );
    }

    private static void assertSymbol(
        DependencyDeprecatedUsageReportComposer.ReportSymbol symbol,
        String note
    ) {
        assertNotNull(symbol);
        assertEquals(note, symbol.getDeprecationNote());
    }

    private static Map<String, DependencyDeprecatedUsageReportComposer.ReportSymbol> bySymbol(
        List<DependencyDeprecatedUsageReportComposer.ReportSymbol> symbols
    ) {
        Map<String, DependencyDeprecatedUsageReportComposer.ReportSymbol> out =
            new LinkedHashMap<String, DependencyDeprecatedUsageReportComposer.ReportSymbol>();
        for (DependencyDeprecatedUsageReportComposer.ReportSymbol symbol : symbols) {
            out.put(symbol.getSymbol(), symbol);
        }
        return out;
    }

    private static Path createVersionArtifacts(
        Path artifactRoot,
        String version,
        String binarySource,
        String sourcesSource,
        JavaCompiler compiler
    ) throws IOException {
        Path jarPath = artifactRoot.resolve(version).resolve("lib-" + version + ".jar");
        createDependencyJar(jarPath, binarySource, compiler);
        if (sourcesSource != null) {
            Path sourcesJar = jarPath
                .getParent()
                .resolve("lib-" + version + "-sources.jar");
            createSourcesJar(
                sourcesJar,
                Collections.singletonMap("org/example/lib/Api.java", sourcesSource)
            );
        }
        return jarPath;
    }

    private static String sourceV09() {
        return (
            "package org.example.lib;\n" +
            "public class Api {\n" +
            "  /** @deprecated use newApi() */\n" +
            "  @Deprecated(since = \"0.9\")\n" +
            "  public void old() {}\n" +
            "  @Deprecated\n" +
            "  public void oldNoDoc() {}\n" +
            "  public void active() {}\n" +
            "}\n"
        );
    }

    private static String sourceV10Binary() {
        return (
            "package org.example.lib;\n" +
            "public class Api {\n" +
            "  /** @deprecated use betterApi() */\n" +
            "  @Deprecated(since = \"0.9\")\n" +
            "  public void old() {}\n" +
            "  @Deprecated\n" +
            "  public void oldNoDoc() {}\n" +
            "  @Deprecated\n" +
            "  public void bytecodeOnly() {}\n" +
            "  @Deprecated\n" +
            "  public static <T extends java.lang.Number> void genericBytecodeOnly(T value) {}\n" +
            "  /** @deprecated use genericReplacement(T) */\n" +
            "  @Deprecated\n" +
            "  public static <T extends java.lang.Number> void genericWithDoc(T value) {}\n" +
            "  public void active() {}\n" +
            "}\n"
        );
    }

    private static String sourceV10Sources() {
        return (
            "package org.example.lib;\n" +
            "public class Api {\n" +
            "  /** @deprecated use betterApi() */\n" +
            "  @Deprecated(since = \"0.9\")\n" +
            "  public void old() {}\n" +
            "  @Deprecated\n" +
            "  public void oldNoDoc() {}\n" +
            "  /** @deprecated use genericReplacement(T) */\n" +
            "  @Deprecated\n" +
            "  public static <T extends java.lang.Number> void genericWithDoc(T value) {}\n" +
            "  public void active() {}\n" +
            "}\n"
        );
    }

    private static String sourceV11Binary() {
        return (
            "package org.example.lib;\n" +
            "public class Api {\n" +
            "  /** @deprecated use newestApi() */\n" +
            "  @Deprecated(since = \"0.9\")\n" +
            "  public void old() {}\n" +
            "  public void oldNoDoc() {}\n" +
            "  @Deprecated\n" +
            "  public void bytecodeOnly() {}\n" +
            "  @Deprecated\n" +
            "  public void onlyInNewer() {}\n" +
            "  public void active() {}\n" +
            "}\n"
        );
    }

    private static String sourceV11Sources() {
        return (
            "package org.example.lib;\n" +
            "public class Api {\n" +
            "  /** @deprecated use newestApi() */\n" +
            "  @Deprecated(since = \"0.9\")\n" +
            "  public void old() {}\n" +
            "  public void oldNoDoc() {}\n" +
            "  /** @deprecated introduced later */\n" +
            "  @Deprecated\n" +
            "  public void onlyInNewer() {}\n" +
            "  public void active() {}\n" +
            "}\n"
        );
    }

    private static void writeUsageReport(Path usageReport, List<String> symbols)
        throws IOException {
        StringBuilder out = new StringBuilder();
        out.append("{\n");
        out.append("  \"usages\": [\n");
        out.append("    {\n");
        out.append("      \"ga\": \"org.example:lib@1.0\",\n");
        out.append("      \"symbols\": [\n");
        for (int i = 0; i < symbols.size(); i++) {
            out
                .append("        \"")
                .append(symbols.get(i))
                .append("\"");
            if (i + 1 < symbols.size()) {
                out.append(',');
            }
            out.append("\n");
        }
        out.append("      ]\n");
        out.append("    }\n");
        out.append("  ]\n");
        out.append("}\n");
        Files.write(usageReport, out.toString().getBytes(StandardCharsets.UTF_8));
    }

    private static void createDependencyJar(
        Path jarPath,
        String source,
        JavaCompiler compiler
    ) throws IOException {
        Path sourceRoot = jarPath.getParent().resolve("src");
        Path sourceFile = sourceRoot.resolve("org/example/lib/Api.java");
        Files.createDirectories(sourceFile.getParent());
        Files.write(sourceFile, source.getBytes(StandardCharsets.UTF_8));

        Path classesDir = jarPath.getParent().resolve("classes");
        Files.createDirectories(classesDir);

        try (
            StandardJavaFileManager fileManager = compiler.getStandardFileManager(
                null,
                null,
                StandardCharsets.UTF_8
            )
        ) {
            Iterable<? extends JavaFileObject> compilationUnits =
                fileManager.getJavaFileObjects(sourceFile.toFile());
            List<String> options = Arrays.asList("-d", classesDir.toString());
            Boolean ok = compiler
                .getTask(null, fileManager, null, options, null, compilationUnits)
                .call();
            assertTrue(Boolean.TRUE.equals(ok), "dependency compilation failed");
        }

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
    }

    private static void createSourcesJar(Path jarPath, Map<String, String> files)
        throws IOException {
        Files.createDirectories(jarPath.getParent());
        try (JarOutputStream out = new JarOutputStream(Files.newOutputStream(jarPath))) {
            for (Map.Entry<String, String> file : files.entrySet()) {
                out.putNextEntry(new JarEntry(file.getKey()));
                out.write(file.getValue().getBytes(StandardCharsets.UTF_8));
                out.closeEntry();
            }
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
