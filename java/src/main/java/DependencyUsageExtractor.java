import com.github.javaparser.JavaParser;
import com.github.javaparser.ParseResult;
import com.github.javaparser.ParserConfiguration;
import com.github.javaparser.ast.CompilationUnit;
import com.github.javaparser.ast.expr.FieldAccessExpr;
import com.github.javaparser.ast.expr.MethodCallExpr;
import com.github.javaparser.ast.expr.ObjectCreationExpr;
import com.github.javaparser.ast.type.Type;
import com.github.javaparser.resolution.declarations.ResolvedConstructorDeclaration;
import com.github.javaparser.resolution.declarations.ResolvedFieldDeclaration;
import com.github.javaparser.resolution.declarations.ResolvedMethodDeclaration;
import com.github.javaparser.resolution.declarations.ResolvedReferenceTypeDeclaration;
import com.github.javaparser.resolution.declarations.ResolvedTypeDeclaration;
import com.github.javaparser.resolution.declarations.ResolvedValueDeclaration;
import com.github.javaparser.resolution.types.ResolvedArrayType;
import com.github.javaparser.resolution.types.ResolvedReferenceType;
import com.github.javaparser.resolution.types.ResolvedType;
import com.github.javaparser.symbolsolver.JavaSymbolSolver;
import com.github.javaparser.symbolsolver.resolution.typesolvers.ClassLoaderTypeSolver;
import com.github.javaparser.symbolsolver.resolution.typesolvers.CombinedTypeSolver;
import com.github.javaparser.symbolsolver.resolution.typesolvers.JarTypeSolver;
import com.github.javaparser.symbolsolver.resolution.typesolvers.JavaParserTypeSolver;
import com.github.javaparser.symbolsolver.resolution.typesolvers.ReflectionTypeSolver;
import java.io.IOException;
import java.net.URL;
import java.net.URLClassLoader;
import java.nio.charset.StandardCharsets;
import java.nio.file.FileVisitResult;
import java.nio.file.FileVisitor;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.SimpleFileVisitor;
import java.nio.file.attribute.BasicFileAttributes;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.Enumeration;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Set;
import java.util.TreeMap;
import java.util.TreeSet;
import java.util.jar.JarEntry;
import java.util.jar.JarFile;

public class DependencyUsageExtractor {
    private static final String UNKNOWN_VERSION = "unknown";
    private static final String UNKNOWN_GROUP_ID = "unknown";
    private static final String UNKNOWN_ARTIFACT_ID = "unknown";
    private static final Set<String> SKIP_DIRECTORIES = new HashSet<String>(
        Arrays.asList(
            ".git",
            ".idea",
            ".gradle",
            ".mvn",
            "target",
            "build",
            "out",
            "tmp",
            "node_modules"
        )
    );

    public Result analyze(Config config) throws IOException {
        Config normalized = Config.normalized(config);
        List<Path> sourceRoots = discoverMainSourceRoots(normalized.getRepoRoot());
        if (sourceRoots.isEmpty()) {
            return new Result(Collections.<UsageGroup>emptyList());
        }

        List<Path> classpathEntries = readClasspathEntries(
            normalized.getClasspathFile()
        );
        List<Path> jarEntries = selectJarEntries(classpathEntries);
        JarClassDependencyIndex classDependencyIndex = new JarClassDependencyIndex(
            jarEntries,
            normalized.getTargetPackages()
        );

        CombinedTypeSolver typeSolver = buildTypeSolver(
            sourceRoots,
            classpathEntries,
            jarEntries
        );
        JavaParser parser = newParser(typeSolver);
        UsageCollector collector = new UsageCollector(
            normalized.getTargetPackages(),
            classDependencyIndex
        );

        for (Path sourceRoot : sourceRoots) {
            List<Path> javaFiles = collectJavaFiles(sourceRoot);
            for (Path javaFile : javaFiles) {
                analyzeJavaFile(parser, javaFile, collector);
            }
        }

        return collector.buildResult();
    }

    static String extractVersionFromJarPath(Path jarPath) {
        return extractDependencyFromJarPath(jarPath).getVersion();
    }

    static DependencyCoordinates extractDependencyFromJarPath(Path jarPath) {
        if (jarPath == null || jarPath.getFileName() == null) {
            return DependencyCoordinates.unknown();
        }
        String fileName = jarPath.getFileName().toString();
        if (!fileName.endsWith(".jar")) {
            return DependencyCoordinates.unknown();
        }

        List<String> parts = new ArrayList<String>();
        for (Path segment : jarPath) {
            parts.add(segment.toString());
        }

        for (int i = 0; i + 6 < parts.size(); i++) {
            if (
                "modules-2".equals(parts.get(i)) &&
                "files-2.1".equals(parts.get(i + 1))
            ) {
                return new DependencyCoordinates(
                    parts.get(i + 2),
                    parts.get(i + 3),
                    parts.get(i + 4)
                );
            }
        }

        int repositoryIndex = -1;
        for (int i = 0; i < parts.size(); i++) {
            if ("repository".equals(parts.get(i))) {
                repositoryIndex = i;
            }
        }

        if (repositoryIndex >= 0 && repositoryIndex + 4 <= parts.size()) {
            int artifactIndex = parts.size() - 3;
            int versionIndex = parts.size() - 2;
            if (artifactIndex > repositoryIndex + 1) {
                String artifact = parts.get(artifactIndex);
                String version = parts.get(versionIndex);
                String expectedPrefix = artifact + "-" + version;
                if (fileName.startsWith(expectedPrefix)) {
                    List<String> groupParts = parts.subList(
                        repositoryIndex + 1,
                        artifactIndex
                    );
                    String groupId = String.join(".", groupParts);
                    if (!groupId.isEmpty()) {
                        return new DependencyCoordinates(groupId, artifact, version);
                    }
                }
            }
        }

        if (parts.size() >= 3) {
            String artifact = parts.get(parts.size() - 3);
            String version = parts.get(parts.size() - 2);
            String expectedPrefix = artifact + "-" + version;
            if (fileName.startsWith(expectedPrefix)) {
                return new DependencyCoordinates(
                    UNKNOWN_GROUP_ID,
                    artifact,
                    version
                );
            }
        }

        return DependencyCoordinates.unknown();
    }

    static String longestMatchingTargetPackage(
        String packageName,
        List<String> targetPackages
    ) {
        if (packageName == null) {
            return null;
        }
        String winner = null;
        for (String targetPackage : targetPackages) {
            if (packageName.startsWith(targetPackage)) {
                if (winner == null || targetPackage.length() > winner.length()) {
                    winner = targetPackage;
                }
            }
        }
        return winner;
    }

    private static CombinedTypeSolver buildTypeSolver(
        List<Path> sourceRoots,
        List<Path> classpathEntries,
        List<Path> jarEntries
    ) {
        CombinedTypeSolver solver = new CombinedTypeSolver();
        solver.add(new ReflectionTypeSolver());

        for (Path sourceRoot : sourceRoots) {
            solver.add(new JavaParserTypeSolver(sourceRoot));
        }

        for (Path jarEntry : jarEntries) {
            try {
                solver.add(new JarTypeSolver(jarEntry));
            } catch (IOException ignored) {
                // Unreadable jars are ignored, extraction proceeds with remaining inputs.
            }
        }

        ClassLoaderTypeSolver classpathSolver = classpathTypeSolver(classpathEntries);
        if (classpathSolver != null) {
            solver.add(classpathSolver);
        }

        return solver;
    }

    private static ClassLoaderTypeSolver classpathTypeSolver(
        List<Path> classpathEntries
    ) {
        LinkedHashSet<URL> urls = new LinkedHashSet<URL>();
        for (Path entry : classpathEntries) {
            if (!Files.exists(entry)) {
                continue;
            }
            try {
                urls.add(entry.toUri().toURL());
            } catch (Exception ignored) {}
        }
        if (urls.isEmpty()) {
            return null;
        }

        URLClassLoader classLoader = new URLClassLoader(
            urls.toArray(new URL[0]),
            DependencyUsageExtractor.class.getClassLoader()
        );
        return new ClassLoaderTypeSolver(classLoader);
    }

    private static JavaParser newParser(CombinedTypeSolver typeSolver) {
        ParserConfiguration config = new ParserConfiguration();
        config.setLanguageLevel(ParserConfiguration.LanguageLevel.BLEEDING_EDGE);
        config.setSymbolResolver(new JavaSymbolSolver(typeSolver));
        return new JavaParser(config);
    }

    private static void analyzeJavaFile(
        JavaParser parser,
        Path javaFile,
        UsageCollector collector
    ) throws IOException {
        ParseResult<CompilationUnit> parseResult = parser.parse(javaFile);
        if (!parseResult.getResult().isPresent()) {
            return;
        }

        CompilationUnit unit = parseResult.getResult().get();

        for (MethodCallExpr methodCall : unit.findAll(MethodCallExpr.class)) {
            try {
                collector.recordMethod(methodCall.resolve());
            } catch (RuntimeException ignored) {}
        }

        for (
            ObjectCreationExpr objectCreation : unit.findAll(
                ObjectCreationExpr.class
            )
        ) {
            try {
                collector.recordConstructor(objectCreation.resolve());
            } catch (RuntimeException ignored) {}
        }

        for (Type typeUsage : unit.findAll(Type.class)) {
            try {
                collector.recordType(typeUsage.resolve());
            } catch (RuntimeException ignored) {}
        }

        for (FieldAccessExpr fieldAccess : unit.findAll(FieldAccessExpr.class)) {
            try {
                ResolvedValueDeclaration resolved = fieldAccess.resolve();
                if (resolved.isField()) {
                    collector.recordField(resolved.asField());
                }
            } catch (RuntimeException ignored) {}
        }
    }

    private static List<Path> readClasspathEntries(Path classpathFile)
        throws IOException {
        if (classpathFile == null || !Files.isRegularFile(classpathFile)) {
            throw new IOException(
                "Classpath file does not exist: " + String.valueOf(classpathFile)
            );
        }

        LinkedHashSet<Path> entries = new LinkedHashSet<Path>();
        List<String> lines = Files.readAllLines(classpathFile, StandardCharsets.UTF_8);
        for (String line : lines) {
            String trimmed = line == null ? "" : line.trim();
            if (trimmed.isEmpty() || trimmed.startsWith("#")) {
                continue;
            }

            Path entry = Path.of(trimmed);
            if (!entry.isAbsolute()) {
                Path baseDir = classpathFile.getParent();
                if (baseDir != null) {
                    entry = baseDir.resolve(entry);
                }
            }
            entries.add(entry.normalize());
        }

        return new ArrayList<Path>(entries);
    }

    private static List<Path> selectJarEntries(List<Path> classpathEntries) {
        List<Path> jarEntries = new ArrayList<Path>();
        for (Path entry : classpathEntries) {
            if (!Files.isRegularFile(entry)) {
                continue;
            }
            String fileName =
                entry.getFileName() == null ? "" : entry.getFileName().toString();
            if (!fileName.endsWith(".jar")) {
                continue;
            }
            jarEntries.add(entry);
        }
        return jarEntries;
    }

    private static List<Path> discoverMainSourceRoots(Path repoRoot)
        throws IOException {
        final List<Path> roots = new ArrayList<Path>();
        FileVisitor<Path> visitor =
            new SimpleFileVisitor<Path>() {
                @Override
                public FileVisitResult preVisitDirectory(
                    Path dir,
                    BasicFileAttributes attrs
                ) {
                    if (!repoRoot.equals(dir)) {
                        String name =
                            dir.getFileName() == null
                                ? ""
                                : dir.getFileName().toString();
                        if (SKIP_DIRECTORIES.contains(name)) {
                            return FileVisitResult.SKIP_SUBTREE;
                        }
                    }

                    if (isMainJavaSourceRoot(dir)) {
                        roots.add(dir);
                        return FileVisitResult.SKIP_SUBTREE;
                    }
                    return FileVisitResult.CONTINUE;
                }
            };

        Files.walkFileTree(repoRoot, visitor);
        Collections.sort(roots);
        return roots;
    }

    private static boolean isMainJavaSourceRoot(Path dir) {
        int count = dir.getNameCount();
        if (count < 3) {
            return false;
        }
        return (
            "src".equals(dir.getName(count - 3).toString()) &&
            "main".equals(dir.getName(count - 2).toString()) &&
            "java".equals(dir.getName(count - 1).toString())
        );
    }

    private static List<Path> collectJavaFiles(Path sourceRoot) throws IOException {
        List<Path> javaFiles = new ArrayList<Path>();
        Files.walkFileTree(
            sourceRoot,
            new SimpleFileVisitor<Path>() {
                @Override
                public FileVisitResult visitFile(
                    Path file,
                    BasicFileAttributes attrs
                ) {
                    if (attrs.isRegularFile() && file.toString().endsWith(".java")) {
                        javaFiles.add(file);
                    }
                    return FileVisitResult.CONTINUE;
                }
            }
        );
        Collections.sort(javaFiles);
        return javaFiles;
    }

    public static final class Config {
        private final Path repoRoot;
        private final Path classpathFile;
        private final List<String> targetPackages;

        public Config(Path repoRoot, Path classpathFile, List<String> targetPackages) {
            this.repoRoot = repoRoot;
            this.classpathFile = classpathFile;
            this.targetPackages = targetPackages;
        }

        public Path getRepoRoot() {
            return repoRoot;
        }

        public Path getClasspathFile() {
            return classpathFile;
        }

        public List<String> getTargetPackages() {
            return targetPackages;
        }

        static Config normalized(Config raw) {
            Objects.requireNonNull(raw, "config must not be null");
            Objects.requireNonNull(raw.repoRoot, "repoRoot must not be null");
            Objects.requireNonNull(
                raw.classpathFile,
                "classpathFile must not be null"
            );
            Objects.requireNonNull(
                raw.targetPackages,
                "targetPackages must not be null"
            );

            List<String> normalizedTargets = new ArrayList<String>();
            for (String targetPackage : raw.targetPackages) {
                if (targetPackage == null) {
                    continue;
                }
                String trimmed = targetPackage.trim();
                if (trimmed.isEmpty()) {
                    continue;
                }
                normalizedTargets.add(trimmed);
            }
            if (!normalizedTargets.isEmpty()) {
                Collections.sort(
                    normalizedTargets,
                    (left, right) -> {
                        int len = Integer.compare(right.length(), left.length());
                        if (len != 0) {
                            return len;
                        }
                        return left.compareTo(right);
                    }
                );
            }

            return new Config(
                raw.repoRoot.toAbsolutePath().normalize(),
                raw.classpathFile.toAbsolutePath().normalize(),
                Collections.unmodifiableList(normalizedTargets)
            );
        }
    }

    public static final class Result {
        private final List<UsageGroup> usages;

        Result(List<UsageGroup> usages) {
            this.usages = usages;
        }

        public List<UsageGroup> getUsages() {
            return usages;
        }
    }

    public static final class UsageGroup {
        private final String ga;
        private final List<String> symbols;

        UsageGroup(String ga, List<String> symbols) {
            this.ga = ga;
            this.symbols = symbols;
        }

        public String getGa() {
            return ga;
        }

        public List<String> getSymbols() {
            return symbols;
        }
    }

    private static final class UsageCollector {
        private final List<String> targetPackages;
        private final JarClassDependencyIndex dependencyIndex;
        private final TreeMap<UsageKey, TreeSet<String>> usageByKey =
            new TreeMap<UsageKey, TreeSet<String>>();

        UsageCollector(
            List<String> targetPackages,
            JarClassDependencyIndex dependencyIndex
        ) {
            this.targetPackages = targetPackages;
            this.dependencyIndex = dependencyIndex;
        }

        void recordMethod(ResolvedMethodDeclaration declaration) {
            ResolvedReferenceTypeDeclaration owner = declaration.declaringType();
            String ownerType = normalizeTypeName(owner.getQualifiedName());
            String symbol =
                ownerType +
                "#" +
                declaration.getName() +
                "(" +
                methodParameters(declaration) +
                ")";
            recordUsage(owner.getPackageName(), ownerType, symbol);
        }

        void recordConstructor(ResolvedConstructorDeclaration declaration) {
            ResolvedReferenceTypeDeclaration owner = declaration.declaringType();
            String ownerType = normalizeTypeName(owner.getQualifiedName());
            String symbol =
                ownerType + "#<init>(" + constructorParameters(declaration) + ")";
            recordUsage(owner.getPackageName(), ownerType, symbol);
        }

        void recordField(ResolvedFieldDeclaration declaration) {
            ResolvedTypeDeclaration owner = declaration.declaringType();
            String ownerType = normalizeTypeName(owner.getQualifiedName());
            String symbol = ownerType + "#" + declaration.getName();
            recordUsage(owner.getPackageName(), ownerType, symbol);
        }

        void recordType(ResolvedType resolvedType) {
            if (!resolvedType.isReferenceType()) {
                return;
            }

            ResolvedReferenceType referenceType = resolvedType.asReferenceType();
            String ownerType = normalizeTypeName(referenceType.getQualifiedName());

            String packageName = null;
            try {
                if (referenceType.getTypeDeclaration().isPresent()) {
                    packageName = referenceType
                        .getTypeDeclaration()
                        .get()
                        .getPackageName();
                    ownerType = normalizeTypeName(
                        referenceType.getTypeDeclaration().get().getQualifiedName()
                    );
                }
            } catch (RuntimeException ignored) {}

            if (packageName == null) {
                packageName = packageFromQualifiedType(ownerType);
            }

            recordUsage(packageName, ownerType, ownerType);
        }

        Result buildResult() {
            List<UsageGroup> usages = new ArrayList<UsageGroup>();
            for (Map.Entry<UsageKey, TreeSet<String>> entry : usageByKey.entrySet()) {
                usages.add(
                    new UsageGroup(
                        entry.getKey().ga,
                        Collections.unmodifiableList(
                            new ArrayList<String>(entry.getValue())
                        )
                    )
                );
            }
            return new Result(Collections.unmodifiableList(usages));
        }

        private void recordUsage(
            String packageName,
            String ownerType,
            String symbolValue
        ) {
            if (packageName == null || packageName.isEmpty()) {
                return;
            }
            if (isJdkPackage(packageName)) {
                return;
            }

            String matchedTarget = null;
            if (targetPackages.isEmpty()) {
                matchedTarget = packageName;
            } else {
                matchedTarget = longestMatchingTargetPackage(
                    packageName,
                    targetPackages
                );
            }
            if (matchedTarget == null) {
                return;
            }

            DependencyCoordinates dependency = dependencyIndex.dependencyForType(
                ownerType
            );
            UsageKey key = new UsageKey(dependency.getGa());
            TreeSet<String> symbols = usageByKey.get(key);
            if (symbols == null) {
                symbols = new TreeSet<String>();
                usageByKey.put(key, symbols);
            }
            symbols.add(symbolValue);
        }

        private static String methodParameters(ResolvedMethodDeclaration declaration) {
            List<String> parameters = new ArrayList<String>();
            for (int i = 0; i < declaration.getNumberOfParams(); i++) {
                parameters.add(describeType(declaration.getParam(i).getType()));
            }
            return String.join(",", parameters);
        }

        private static String constructorParameters(
            ResolvedConstructorDeclaration declaration
        ) {
            List<String> parameters = new ArrayList<String>();
            for (int i = 0; i < declaration.getNumberOfParams(); i++) {
                parameters.add(describeType(declaration.getParam(i).getType()));
            }
            return String.join(",", parameters);
        }
    }

    private static final class JarClassDependencyIndex {
        private final List<String> targetPackages;
        private final Map<String, DependencyCoordinates> dependencyByType =
            new HashMap<String, DependencyCoordinates>();

        JarClassDependencyIndex(List<Path> jarEntries, List<String> targetPackages)
            throws IOException {
            this.targetPackages = targetPackages;
            indexJars(jarEntries);
        }

        DependencyCoordinates dependencyForType(String typeName) {
            if (typeName == null || typeName.isEmpty()) {
                return DependencyCoordinates.unknown();
            }
            String normalized = normalizeTypeName(typeName);
            DependencyCoordinates dependency = dependencyByType.get(normalized);
            if (dependency == null) {
                return DependencyCoordinates.unknown();
            }
            return dependency;
        }

        private void indexJars(List<Path> jarEntries) throws IOException {
            for (Path jarPath : jarEntries) {
                DependencyCoordinates dependency = extractDependencyFromJarPath(
                    jarPath
                );
                indexSingleJar(jarPath, dependency);
            }
        }

        private void indexSingleJar(Path jarPath, DependencyCoordinates dependency)
            throws IOException {
            try (JarFile jarFile = new JarFile(jarPath.toFile())) {
                Enumeration<JarEntry> entries = jarFile.entries();
                while (entries.hasMoreElements()) {
                    JarEntry entry = entries.nextElement();
                    if (entry.isDirectory()) {
                        continue;
                    }
                    String name = entry.getName();
                    if (!name.endsWith(".class")) {
                        continue;
                    }
                    String className = name
                        .substring(0, name.length() - ".class".length())
                        .replace('/', '.')
                        .replace('$', '.');
                    if (!matchesAnyTargetPrefix(className, targetPackages)) {
                        continue;
                    }
                    if (!dependencyByType.containsKey(className)) {
                        dependencyByType.put(className, dependency);
                    }
                }
            } catch (IOException ignored) {
                // Unreadable jars are ignored in the class-dependency index.
            }
        }
    }

    private static final class UsageKey implements Comparable<UsageKey> {
        private final String ga;

        UsageKey(String ga) {
            this.ga = ga;
        }

        @Override
        public int compareTo(UsageKey other) {
            return ga.compareTo(other.ga);
        }

        @Override
        public boolean equals(Object obj) {
            if (this == obj) {
                return true;
            }
            if (!(obj instanceof UsageKey)) {
                return false;
            }
            UsageKey other = (UsageKey) obj;
            return ga.equals(other.ga);
        }

        @Override
        public int hashCode() {
            return Objects.hash(ga);
        }
    }

    static final class DependencyCoordinates {
        private final String groupId;
        private final String artifactId;
        private final String version;

        DependencyCoordinates(String groupId, String artifactId, String version) {
            this.groupId = safe(groupId, UNKNOWN_GROUP_ID);
            this.artifactId = safe(artifactId, UNKNOWN_ARTIFACT_ID);
            this.version = safe(version, UNKNOWN_VERSION);
        }

        static DependencyCoordinates unknown() {
            return new DependencyCoordinates(
                UNKNOWN_GROUP_ID,
                UNKNOWN_ARTIFACT_ID,
                UNKNOWN_VERSION
            );
        }

        String getGroupId() {
            return groupId;
        }

        String getArtifactId() {
            return artifactId;
        }

        String getVersion() {
            return version;
        }

        String getGa() {
            return groupId + ":" + artifactId + "@" + version;
        }

        private static String safe(String value, String fallback) {
            if (value == null) {
                return fallback;
            }
            String trimmed = value.trim();
            if (trimmed.isEmpty()) {
                return fallback;
            }
            return trimmed;
        }
    }

    private static boolean matchesAnyTargetPrefix(
        String className,
        List<String> targetPackages
    ) {
        if (targetPackages.isEmpty()) {
            return true;
        }
        for (String targetPackage : targetPackages) {
            if (className.equals(targetPackage)) {
                return true;
            }
            if (className.startsWith(targetPackage + ".")) {
                return true;
            }
        }
        return false;
    }

    private static String normalizeTypeName(String typeName) {
        return typeName == null ? "" : typeName.replace('$', '.');
    }

    private static String packageFromQualifiedType(String typeName) {
        if (typeName == null || typeName.isEmpty()) {
            return "";
        }
        int index = typeName.lastIndexOf('.');
        if (index <= 0) {
            return "";
        }
        return typeName.substring(0, index);
    }

    private static String describeType(ResolvedType resolvedType) {
        if (resolvedType == null) {
            return "unknown";
        }
        if (resolvedType.isPrimitive()) {
            return resolvedType.describe();
        }
        if (resolvedType.isArray()) {
            ResolvedArrayType arrayType = resolvedType.asArrayType();
            return describeType(arrayType.getComponentType()) + "[]";
        }
        if (resolvedType.isReferenceType()) {
            return normalizeTypeName(resolvedType.asReferenceType().getQualifiedName());
        }
        return resolvedType.describe();
    }

    private static boolean isJdkPackage(String packageName) {
        return (
            "java".equals(packageName) ||
            packageName.startsWith("java.") ||
            packageName.startsWith("javax.") ||
            packageName.startsWith("jdk.")
        );
    }
}
