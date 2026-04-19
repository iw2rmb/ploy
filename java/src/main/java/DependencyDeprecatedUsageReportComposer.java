import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.io.IOException;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Comparator;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Set;
import java.util.stream.Collectors;
import org.eclipse.aether.RepositorySystem;
import org.eclipse.aether.RepositorySystemSession;
import org.eclipse.aether.artifact.Artifact;
import org.eclipse.aether.artifact.DefaultArtifact;
import org.eclipse.aether.repository.RemoteRepository;
import org.eclipse.aether.resolution.ArtifactRequest;
import org.eclipse.aether.resolution.ArtifactResult;

public final class DependencyDeprecatedUsageReportComposer {
    private static final ObjectMapper JSON = new ObjectMapper();

    private final SourceDeprecationScanner sourceScanner =
        new SourceDeprecationScanner();

    public List<ReportGroup> compose(Config config) throws IOException {
        Config normalized = Config.normalized(config);
        JsonNode usageRoot = JSON.readTree(normalized.getUsageReportFile().toFile());
        if (usageRoot == null || !usageRoot.isObject()) {
            throw new IOException(
                "Usage report must be a JSON object with .usages: " +
                normalized.getUsageReportFile()
            );
        }

        JsonNode usagesNode = usageRoot.get("usages");
        if (usagesNode == null || !usagesNode.isArray()) {
            throw new IOException(
                "Usage report must contain array field .usages: " +
                normalized.getUsageReportFile()
            );
        }

        Map<String, List<Path>> classpathJarsByGaVersion = classpathBinaryJarsByGaVersion(
            normalized.getClasspathFile()
        );

        ResolverBridge resolverBridge = ResolverBridge.tryCreate(
            normalized.getRepoUrl()
        );

        List<ReportGroup> groups = new ArrayList<ReportGroup>();
        int usageIndex = 0;
        int usageTotal = usagesNode.size();
        for (JsonNode usageNode : usagesNode) {
            usageIndex++;
            if (!usageNode.isObject()) {
                continue;
            }

            UsageGroupInput usageInput = UsageGroupInput.fromJson(usageNode);
            if (usageInput == null) {
                continue;
            }
            emitPackageProgressEvent(normalized, usageInput, usageIndex, usageTotal);

            List<Path> currentBinaryJars = classpathJarsByGaVersion.getOrDefault(
                usageInput.gaVersion(),
                Collections.emptyList()
            );
            VersionArtifactResolver artifactResolver = new VersionArtifactResolver(
                usageInput.groupId,
                usageInput.artifactId,
                usageInput.version,
                currentBinaryJars,
                resolverBridge
            );

            VersionSnapshot currentSnapshot = buildCurrentSnapshot(
                artifactResolver,
                usageInput.version
            );
            if (currentSnapshot == null) {
                currentSnapshot = VersionSnapshot.empty(usageInput.version);
            }
            final VersionSnapshot snapshotForFilter = currentSnapshot;

            List<String> sortedSymbols = usageInput.usedSymbolsCanonical
                .stream()
                .filter(symbol -> {
                    try {
                        return snapshotForFilter.isDeprecated(symbol);
                    } catch (IOException ignored) {
                        return false;
                    }
                })
                .sorted()
                .collect(Collectors.toCollection(ArrayList::new));
            if (sortedSymbols.isEmpty()) {
                continue;
            }

            List<ReportSymbol> reportSymbols = new ArrayList<ReportSymbol>();
            for (String symbol : sortedSymbols) {
                String note = null;
                SourceDeprecationScanner.DeprecationEntry sourceEntry =
                    currentSnapshot.sourceEntry(symbol);
                if (sourceEntry != null) {
                    note = sourceEntry.getNote();
                }
                reportSymbols.add(
                    new ReportSymbol(symbol, note)
                );
            }

            groups.add(
                new ReportGroup(
                    usageInput.gaVersion(),
                    reportSymbols
                )
            );
        }

        Collections.sort(groups, Comparator.comparing(ReportGroup::getGa));
        return groups;
    }

    private static void emitPackageProgressEvent(
        Config config,
        UsageGroupInput usageInput,
        int index,
        int total
    ) {
        if (!config.isEmitProgressEventsToStdout()) {
            return;
        }
        try {
            ObjectNode event = JSON.createObjectNode();
            event.put("event", "deprecated_usage.package.start");
            event.put("index", index);
            event.put("total", total);
            event.put("ga", usageInput.gaVersion());
            System.out.println(JSON.writeValueAsString(event));
            System.out.flush();
        } catch (Exception ignored) {
            // Progress events are best-effort only.
        }
    }

    public static String toJson(List<ReportGroup> groups) throws IOException {
        ArrayNode root = JSON.createArrayNode();
        for (ReportGroup group : groups) {
            ObjectNode groupNode = root.addObject();
            groupNode.put("ga", group.getGa());

            ArrayNode symbolsNode = groupNode.putArray("symbols");
            for (ReportSymbol symbol : group.getSymbols()) {
                ObjectNode symbolNode = symbolsNode.addObject();
                symbolNode.put("symbol", symbol.getSymbol());
                if (symbol.getDeprecationNote() == null) {
                    symbolNode.putNull("deprecation_note");
                } else {
                    symbolNode.put(
                        "deprecation_note",
                        symbol.getDeprecationNote()
                    );
                }
            }
        }
        return JSON.writerWithDefaultPrettyPrinter().writeValueAsString(root) + "\n";
    }

    private VersionSnapshot buildCurrentSnapshot(
        VersionArtifactResolver artifactResolver,
        String currentVersion
    ) throws IOException {
        List<Path> binaryJars = artifactResolver.binaryJarsForVersion(currentVersion);
        Path sourcesJar = artifactResolver.sourcesJarForVersion(currentVersion);
        SourceDeprecationScanner.SourceDeprecationCatalog sourceCatalog =
            sourceScanner.scanSourcesJar(sourcesJar);

        BytecodeDeprecationScanner bytecodeScanner = null;
        if (!binaryJars.isEmpty()) {
            bytecodeScanner = new BytecodeDeprecationScanner(binaryJars);
        }
        return new VersionSnapshot(currentVersion, sourceCatalog, bytecodeScanner);
    }

    private static boolean isBlank(String value) {
        return value == null || value.trim().isEmpty();
    }

    private static Map<String, List<Path>> classpathBinaryJarsByGaVersion(
        Path classpathFile
    ) throws IOException {
        Map<String, List<Path>> byGaVersion =
            new LinkedHashMap<String, List<Path>>();

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
            entry = entry.normalize();

            if (!Files.isRegularFile(entry)) {
                continue;
            }
            String fileName = entry.getFileName() == null
                ? ""
                : entry.getFileName().toString();
            if (!fileName.endsWith(".jar") || fileName.endsWith("-sources.jar")) {
                continue;
            }

            DependencyUsageExtractor.DependencyCoordinates coords =
                DependencyUsageExtractor.extractDependencyFromJarPath(entry);
            if (
                isBlank(coords.getGroupId()) ||
                isBlank(coords.getArtifactId()) ||
                isBlank(coords.getVersion())
            ) {
                continue;
            }

            String key = gaVersion(
                coords.getGroupId(),
                coords.getArtifactId(),
                coords.getVersion()
            );
            byGaVersion.computeIfAbsent(key, ignored -> new ArrayList<Path>()).add(entry);
        }

        for (Map.Entry<String, List<Path>> entry : byGaVersion.entrySet()) {
            List<Path> deduped = new ArrayList<Path>(
                new LinkedHashSet<Path>(entry.getValue())
            );
            Collections.sort(deduped);
            entry.setValue(Collections.unmodifiableList(deduped));
        }

        return byGaVersion;
    }

    private static String gaVersion(String groupId, String artifactId, String version) {
        return groupId + ":" + artifactId + "@" + version;
    }

    private static final class UsageGroupInput {
        private final String groupId;
        private final String artifactId;
        private final String version;
        private final Set<String> usedSymbolsCanonical;

        private UsageGroupInput(
            String groupId,
            String artifactId,
            String version,
            Set<String> usedSymbolsCanonical
        ) {
            this.groupId = groupId;
            this.artifactId = artifactId;
            this.version = version;
            this.usedSymbolsCanonical = usedSymbolsCanonical;
        }

        static UsageGroupInput fromJson(JsonNode usageNode) {
            String ga = text(usageNode, "ga");
            String[] parsedGa = parseGaVersion(ga);
            if (parsedGa == null) {
                return null;
            }
            String groupId = parsedGa[0];
            String artifactId = parsedGa[1];
            String version = parsedGa[2];

            JsonNode symbolsNode = usageNode.get("symbols");
            Set<String> usedSymbolsCanonical = new LinkedHashSet<String>();
            if (symbolsNode != null && symbolsNode.isArray()) {
                for (JsonNode symbolNode : symbolsNode) {
                    if (!symbolNode.isTextual()) {
                        continue;
                    }
                    SymbolRef symbol = SymbolRef.parse(symbolNode.asText());
                    if (symbol == null) {
                        continue;
                    }
                    usedSymbolsCanonical.add(symbol.toCanonicalSymbol());
                }
            }
            return new UsageGroupInput(
                groupId,
                artifactId,
                version,
                Collections.unmodifiableSet(usedSymbolsCanonical)
            );
        }

        private static String[] parseGaVersion(String ga) {
            if (isBlank(ga)) {
                return null;
            }
            int atIndex = ga.lastIndexOf('@');
            int colonIndex = ga.indexOf(':');
            if (atIndex <= 0 || colonIndex <= 0 || colonIndex >= atIndex - 1) {
                return null;
            }
            String groupId = ga.substring(0, colonIndex).trim();
            String artifactId = ga.substring(colonIndex + 1, atIndex).trim();
            String version = ga.substring(atIndex + 1).trim();
            if (isBlank(groupId) || isBlank(artifactId) || isBlank(version)) {
                return null;
            }
            return new String[] { groupId, artifactId, version };
        }

        String gaVersion() {
            return DependencyDeprecatedUsageReportComposer.gaVersion(
                groupId,
                artifactId,
                version
            );
        }
    }

    private static String text(JsonNode node, String field) {
        if (node == null) {
            return "";
        }
        JsonNode value = node.get(field);
        if (value == null || value.isNull()) {
            return "";
        }
        return value.asText("").trim();
    }

    public static final class Config {
        private final Path usageReportFile;
        private final Path classpathFile;
        private final String repoUrl;
        private final boolean emitProgressEventsToStdout;

        public Config(Path usageReportFile, Path classpathFile) {
            this(usageReportFile, classpathFile, null, false);
        }

        public Config(Path usageReportFile, Path classpathFile, String repoUrl) {
            this(usageReportFile, classpathFile, repoUrl, false);
        }

        public Config(
            Path usageReportFile,
            Path classpathFile,
            String repoUrl,
            boolean emitProgressEventsToStdout
        ) {
            this.usageReportFile = usageReportFile;
            this.classpathFile = classpathFile;
            this.repoUrl = repoUrl;
            this.emitProgressEventsToStdout = emitProgressEventsToStdout;
        }

        public Path getUsageReportFile() {
            return usageReportFile;
        }

        public Path getClasspathFile() {
            return classpathFile;
        }

        public String getRepoUrl() {
            return repoUrl;
        }

        public boolean isEmitProgressEventsToStdout() {
            return emitProgressEventsToStdout;
        }

        static Config normalized(Config raw) {
            Objects.requireNonNull(raw, "config must not be null");
            Objects.requireNonNull(
                raw.usageReportFile,
                "usageReportFile must not be null"
            );
            Objects.requireNonNull(raw.classpathFile, "classpathFile must not be null");
            return new Config(
                raw.usageReportFile.toAbsolutePath().normalize(),
                raw.classpathFile.toAbsolutePath().normalize(),
                isBlank(raw.repoUrl) ? null : raw.repoUrl.trim(),
                raw.emitProgressEventsToStdout
            );
        }
    }

    public static final class ReportGroup {
        private final String ga;
        private final List<ReportSymbol> symbols;

        ReportGroup(String ga, List<ReportSymbol> symbols) {
            this.ga = ga;
            this.symbols = Collections.unmodifiableList(new ArrayList<ReportSymbol>(symbols));
        }

        public String getGa() {
            return ga;
        }

        public List<ReportSymbol> getSymbols() {
            return symbols;
        }
    }

    public static final class ReportSymbol {
        private final String symbol;
        private final String deprecationNote;

        ReportSymbol(String symbol, String deprecationNote) {
            this.symbol = symbol;
            this.deprecationNote = deprecationNote;
        }

        public String getSymbol() {
            return symbol;
        }

        public String getDeprecationNote() {
            return deprecationNote;
        }
    }

    enum SymbolKind {
        TYPE,
        FIELD,
        METHOD,
        CONSTRUCTOR
    }

    static final class SymbolRef {
        private final String original;
        private final String ownerClass;
        private final String memberName;
        private final List<String> parameterTypes;
        private final SymbolKind kind;

        private SymbolRef(
            String original,
            String ownerClass,
            String memberName,
            List<String> parameterTypes,
            SymbolKind kind
        ) {
            this.original = original;
            this.ownerClass = ownerClass;
            this.memberName = memberName;
            this.parameterTypes = parameterTypes;
            this.kind = kind;
        }

        static SymbolRef parse(String symbol) {
            if (symbol == null) {
                return null;
            }
            String trimmed = symbol.trim();
            if (trimmed.isEmpty()) {
                return null;
            }

            int hash = trimmed.indexOf('#');
            if (hash < 0) {
                return new SymbolRef(
                    trimmed,
                    normalizeClassName(trimmed),
                    "",
                    Collections.emptyList(),
                    SymbolKind.TYPE
                );
            }

            if (hash == 0 || hash + 1 >= trimmed.length()) {
                return null;
            }

            String owner = normalizeClassName(trimmed.substring(0, hash));
            String tail = trimmed.substring(hash + 1).trim();
            if (owner.isEmpty() || tail.isEmpty()) {
                return null;
            }

            int open = tail.indexOf('(');
            int close = tail.lastIndexOf(')');
            if (open >= 0 && close > open) {
                String name = tail.substring(0, open).trim();
                String rawParams = tail.substring(open + 1, close).trim();
                List<String> params = parseParams(rawParams);
                SymbolKind kind = "<init>".equals(name)
                    ? SymbolKind.CONSTRUCTOR
                    : SymbolKind.METHOD;
                return new SymbolRef(trimmed, owner, name, params, kind);
            }

            return new SymbolRef(
                trimmed,
                owner,
                tail,
                Collections.emptyList(),
                SymbolKind.FIELD
            );
        }

        private static List<String> parseParams(String rawParams) {
            if (rawParams.isEmpty()) {
                return Collections.emptyList();
            }
            String[] parts = rawParams.split(",");
            List<String> params = new ArrayList<String>(parts.length);
            for (String part : parts) {
                String trimmed = part.trim();
                if (!trimmed.isEmpty()) {
                    params.add(normalizeClassName(trimmed));
                }
            }
            return Collections.unmodifiableList(params);
        }

        String methodOrConstructorKey() {
            return memberName + "(" + String.join(",", parameterTypes) + ")";
        }

        String toCanonicalSymbol() {
            switch (kind) {
                case TYPE:
                    return ownerClass;
                case FIELD:
                    return ownerClass + "#" + memberName;
                case METHOD:
                case CONSTRUCTOR:
                    return ownerClass + "#" + methodOrConstructorKey();
                default:
                    return original;
            }
        }

        private static String normalizeClassName(String value) {
            if (value == null) {
                return "";
            }
            return value.trim().replace('$', '.');
        }

        String getOwnerClass() {
            return ownerClass;
        }

        String getMemberName() {
            return memberName;
        }

        List<String> getParameterTypes() {
            return parameterTypes;
        }

        SymbolKind getKind() {
            return kind;
        }
    }

    private static final class VersionSnapshot {
        private final String version;
        private final SourceDeprecationScanner.SourceDeprecationCatalog sourceCatalog;
        private final BytecodeDeprecationScanner bytecodeScanner;

        private VersionSnapshot(
            String version,
            SourceDeprecationScanner.SourceDeprecationCatalog sourceCatalog,
            BytecodeDeprecationScanner bytecodeScanner
        ) {
            this.version = version;
            this.sourceCatalog = sourceCatalog;
            this.bytecodeScanner = bytecodeScanner;
        }

        static VersionSnapshot empty(String version) {
            return new VersionSnapshot(
                version,
                SourceDeprecationScanner.SourceDeprecationCatalog.empty(),
                null
            );
        }

        Set<String> sourceDeprecatedSymbols() {
            return sourceCatalog.symbols();
        }

        Set<String> bytecodeDeprecatedSymbols() throws IOException {
            if (bytecodeScanner == null) {
                return Collections.emptySet();
            }
            return bytecodeScanner.allDeprecatedSymbols();
        }

        boolean isDeprecated(String symbol) throws IOException {
            if (sourceCatalog.hasSymbol(symbol)) {
                return true;
            }
            if (bytecodeScanner != null && bytecodeScanner.isDeprecatedCanonical(symbol)) {
                return true;
            }
            return false;
        }

        SourceDeprecationScanner.DeprecationEntry sourceEntry(String symbol) {
            return sourceCatalog.get(symbol);
        }
    }

    private static final class VersionArtifactResolver {
        private final String groupId;
        private final String artifactId;
        private final String currentVersion;
        private final List<Path> currentBinaryJars;
        private final ResolverBridge resolverBridge;

        private final List<Path> localArtifactRoots;
        private final Map<String, List<Path>> binaryJarsByVersion =
            new HashMap<String, List<Path>>();
        private final Map<String, Path> sourcesJarByVersion =
            new HashMap<String, Path>();

        private VersionArtifactResolver(
            String groupId,
            String artifactId,
            String currentVersion,
            List<Path> currentBinaryJars,
            ResolverBridge resolverBridge
        ) {
            this.groupId = groupId;
            this.artifactId = artifactId;
            this.currentVersion = currentVersion;
            this.currentBinaryJars = currentBinaryJars;
            this.resolverBridge = resolverBridge;
            this.localArtifactRoots = detectLocalArtifactRoots(
                currentBinaryJars,
                artifactId,
                currentVersion
            );
        }

        List<Path> binaryJarsForVersion(String version) {
            if (binaryJarsByVersion.containsKey(version)) {
                return binaryJarsByVersion.get(version);
            }

            List<Path> jars = new ArrayList<Path>();
            if (Objects.equals(currentVersion, version) && !currentBinaryJars.isEmpty()) {
                jars.addAll(currentBinaryJars);
            }

            if (jars.isEmpty()) {
                for (Path root : localArtifactRoots) {
                    Path versionDir = root.resolve(version);
                    jars.addAll(findBinaryJarsInVersionDir(versionDir, artifactId, version));
                }
            }

            if (jars.isEmpty() && resolverBridge != null) {
                Path remote = resolverBridge.resolveBinaryJar(groupId, artifactId, version);
                if (remote != null && Files.isRegularFile(remote)) {
                    jars.add(remote);
                }
            }

            List<Path> normalized = new ArrayList<Path>(new LinkedHashSet<Path>(jars));
            Collections.sort(normalized);
            List<Path> immutable = Collections.unmodifiableList(normalized);
            binaryJarsByVersion.put(version, immutable);
            return immutable;
        }

        Path sourcesJarForVersion(String version) {
            if (sourcesJarByVersion.containsKey(version)) {
                return sourcesJarByVersion.get(version);
            }

            Path sources = null;
            for (Path jar : binaryJarsForVersion(version)) {
                Path parent = jar.getParent();
                if (parent == null) {
                    continue;
                }
                Path direct = parent.resolve(artifactId + "-" + version + "-sources.jar");
                if (Files.isRegularFile(direct)) {
                    sources = direct;
                    break;
                }
            }

            if (sources == null) {
                for (Path root : localArtifactRoots) {
                    Path versionDir = root.resolve(version);
                    Path candidate = findSourcesJarInVersionDir(
                        versionDir,
                        artifactId,
                        version
                    );
                    if (candidate != null) {
                        sources = candidate;
                        break;
                    }
                }
            }

            if (sources == null && resolverBridge != null) {
                Path remote = resolverBridge.resolveSourcesJar(
                    groupId,
                    artifactId,
                    version
                );
                if (remote != null && Files.isRegularFile(remote)) {
                    sources = remote;
                }
            }

            sourcesJarByVersion.put(version, sources);
            return sources;
        }

        private static List<Path> detectLocalArtifactRoots(
            List<Path> jars,
            String artifactId,
            String currentVersion
        ) {
            List<Path> roots = new ArrayList<Path>();
            for (Path jar : jars) {
                if (jar == null) {
                    continue;
                }
                Path root = m2ArtifactRoot(jar, artifactId, currentVersion);
                if (root == null) {
                    root = gradleArtifactRoot(jar, artifactId, currentVersion);
                }
                if (root != null && Files.isDirectory(root)) {
                    roots.add(root);
                }
            }
            List<Path> deduped = new ArrayList<Path>(new LinkedHashSet<Path>(roots));
            Collections.sort(deduped);
            return deduped;
        }

        private static Path m2ArtifactRoot(
            Path jar,
            String artifactId,
            String currentVersion
        ) {
            Path versionDir = jar.getParent();
            if (versionDir == null || !currentVersion.equals(versionDir.getFileName().toString())) {
                return null;
            }
            Path artifactDir = versionDir.getParent();
            if (artifactDir == null || !artifactId.equals(artifactDir.getFileName().toString())) {
                return null;
            }
            return artifactDir;
        }

        private static Path gradleArtifactRoot(
            Path jar,
            String artifactId,
            String currentVersion
        ) {
            Path hashDir = jar.getParent();
            if (hashDir == null) {
                return null;
            }
            Path versionDir = hashDir.getParent();
            if (versionDir == null || !currentVersion.equals(versionDir.getFileName().toString())) {
                return null;
            }
            Path artifactDir = versionDir.getParent();
            if (artifactDir == null || !artifactId.equals(artifactDir.getFileName().toString())) {
                return null;
            }
            return artifactDir;
        }

        private static List<Path> findBinaryJarsInVersionDir(
            Path versionDir,
            String artifactId,
            String version
        ) {
            if (!Files.isDirectory(versionDir)) {
                return Collections.emptyList();
            }
            String prefix = artifactId + "-" + version;
            List<Path> jars = new ArrayList<Path>();
            try (java.util.stream.Stream<Path> stream = Files.walk(versionDir, 2)) {
                stream
                    .filter(Files::isRegularFile)
                    .filter(path -> path.getFileName().toString().endsWith(".jar"))
                    .filter(path -> {
                        String fileName = path.getFileName().toString();
                        if (fileName.endsWith("-sources.jar")) {
                            return false;
                        }
                        return fileName.startsWith(prefix);
                    })
                    .forEach(jars::add);
            } catch (IOException ignored) {
                return Collections.emptyList();
            }
            Collections.sort(jars);
            return jars;
        }

        private static Path findSourcesJarInVersionDir(
            Path versionDir,
            String artifactId,
            String version
        ) {
            if (!Files.isDirectory(versionDir)) {
                return null;
            }
            String expected = artifactId + "-" + version + "-sources.jar";
            try (java.util.stream.Stream<Path> stream = Files.walk(versionDir, 2)) {
                return stream
                    .filter(Files::isRegularFile)
                    .filter(path -> expected.equals(path.getFileName().toString()))
                    .findFirst()
                    .orElse(null);
            } catch (IOException ignored) {
                return null;
            }
        }
    }

    private static final class ResolverBridge {
        private final RepositorySystem system;
        private final RepositorySystemSession session;
        private final List<RemoteRepository> repositories;

        private ResolverBridge(
            RepositorySystem system,
            RepositorySystemSession session,
            List<RemoteRepository> repositories
        ) {
            this.system = system;
            this.session = session;
            this.repositories = repositories;
        }

        static ResolverBridge tryCreate(String repoUrl) {
            try {
                Method method = JapicmpRemovalJavadocEnricherCli.class.getDeclaredMethod(
                    "newResolverContext",
                    String.class
                );
                method.setAccessible(true);
                Object resolverContext = method.invoke(null, repoUrl);
                if (resolverContext == null) {
                    return null;
                }

                Field systemField = resolverContext
                    .getClass()
                    .getDeclaredField("system");
                Field sessionField = resolverContext
                    .getClass()
                    .getDeclaredField("session");
                Field repositoriesField = resolverContext
                    .getClass()
                    .getDeclaredField("repositories");
                systemField.setAccessible(true);
                sessionField.setAccessible(true);
                repositoriesField.setAccessible(true);

                RepositorySystem system = (RepositorySystem) systemField.get(
                    resolverContext
                );
                RepositorySystemSession session =
                    (RepositorySystemSession) sessionField.get(resolverContext);
                @SuppressWarnings("unchecked")
                List<RemoteRepository> repositories = (List<RemoteRepository>) repositoriesField.get(
                    resolverContext
                );

                if (system == null || session == null || repositories == null) {
                    return null;
                }

                return new ResolverBridge(system, session, repositories);
            } catch (Exception ignored) {
                return null;
            }
        }

        Path resolveBinaryJar(String groupId, String artifactId, String version) {
            return resolveArtifact(groupId, artifactId, "", "jar", version);
        }

        Path resolveSourcesJar(String groupId, String artifactId, String version) {
            return resolveArtifact(groupId, artifactId, "sources", "jar", version);
        }

        private Path resolveArtifact(
            String groupId,
            String artifactId,
            String classifier,
            String extension,
            String version
        ) {
            try {
                Artifact artifact = new DefaultArtifact(
                    groupId,
                    artifactId,
                    classifier,
                    extension,
                    version
                );
                ArtifactRequest request = new ArtifactRequest();
                request.setArtifact(artifact);
                request.setRepositories(repositories);
                ArtifactResult result = system.resolveArtifact(session, request);
                java.io.File file = result.getArtifact().getFile();
                if (file == null || !file.isFile()) {
                    return null;
                }
                return file.toPath();
            } catch (Exception ignored) {
                return null;
            }
        }
    }
}
