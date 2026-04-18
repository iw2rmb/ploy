import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.github.javaparser.JavaParser;
import com.github.javaparser.ParseResult;
import com.github.javaparser.ParserConfiguration;
import com.github.javaparser.ast.CompilationUnit;
import com.github.javaparser.ast.NodeList;
import com.github.javaparser.ast.ImportDeclaration;
import com.github.javaparser.ast.body.BodyDeclaration;
import com.github.javaparser.ast.body.ConstructorDeclaration;
import com.github.javaparser.ast.body.FieldDeclaration;
import com.github.javaparser.ast.body.MethodDeclaration;
import com.github.javaparser.ast.body.TypeDeclaration;
import com.github.javaparser.ast.comments.JavadocComment;
import com.github.javaparser.ast.nodeTypes.NodeWithJavadoc;
import com.github.javaparser.ast.type.PrimitiveType;
import com.github.javaparser.ast.type.Type;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.Enumeration;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Optional;
import java.util.Set;
import java.util.jar.JarEntry;
import java.util.jar.JarFile;
import javax.xml.parsers.DocumentBuilderFactory;
import org.apache.maven.artifact.versioning.ComparableVersion;
import org.apache.maven.repository.internal.MavenRepositorySystemUtils;
import org.eclipse.aether.DefaultRepositorySystemSession;
import org.eclipse.aether.RepositorySystem;
import org.eclipse.aether.RepositorySystemSession;
import org.eclipse.aether.artifact.Artifact;
import org.eclipse.aether.artifact.DefaultArtifact;
import org.eclipse.aether.connector.basic.BasicRepositoryConnectorFactory;
import org.eclipse.aether.impl.DefaultServiceLocator;
import org.eclipse.aether.repository.Authentication;
import org.eclipse.aether.repository.LocalRepository;
import org.eclipse.aether.repository.Proxy;
import org.eclipse.aether.repository.RemoteRepository;
import org.eclipse.aether.resolution.ArtifactRequest;
import org.eclipse.aether.resolution.ArtifactResult;
import org.eclipse.aether.resolution.VersionRangeRequest;
import org.eclipse.aether.resolution.VersionRangeResult;
import org.eclipse.aether.spi.connector.RepositoryConnectorFactory;
import org.eclipse.aether.spi.connector.transport.TransporterFactory;
import org.eclipse.aether.transport.file.FileTransporterFactory;
import org.eclipse.aether.transport.http.HttpTransporterFactory;
import org.eclipse.aether.util.repository.AuthenticationBuilder;
import org.eclipse.aether.util.repository.DefaultAuthenticationSelector;
import org.eclipse.aether.util.repository.DefaultMirrorSelector;
import org.eclipse.aether.util.repository.DefaultProxySelector;
import org.eclipse.aether.version.Version;
import org.w3c.dom.Document;
import org.w3c.dom.Element;

public final class JapicmpRemovalJavadocEnricherCli {
    private static final ObjectMapper JSON = new ObjectMapper();
    private static final String CENTRAL_ID = "central";
    private static final String DEFAULT_REPO_URL =
        "https://repo.maven.apache.org/maven2/";
    private static final Set<String> JAVA_LANG_TYPES = new HashSet<String>(
        Arrays.asList(
            "Boolean",
            "Byte",
            "Character",
            "Class",
            "Comparable",
            "Double",
            "Enum",
            "Exception",
            "Float",
            "Integer",
            "Iterable",
            "Long",
            "Number",
            "Object",
            "RuntimeException",
            "Short",
            "String",
            "Throwable",
            "Void"
        )
    );

    private JapicmpRemovalJavadocEnricherCli() {}

    public static void main(String[] args) throws Exception {
        CliOptions options;
        try {
            options = CliOptions.parse(args);
        } catch (IllegalArgumentException ex) {
            System.err.println(ex.getMessage());
            printUsage();
            System.exit(1);
            return;
        }

        ArrayNode removals = readArray(options.getRemovalsFile());
        ArrayNode enriched = enrichWithResolver(
            removals,
            options.getGa(),
            options.getFromVersion(),
            options.getToVersion(),
            options.getRepoUrl()
        );
        writeArray(options.getOutputFile(), enriched);
    }

    static ArrayNode enrichWithResolver(
        ArrayNode removals,
        String ga,
        String fromVersion,
        String toVersion,
        String repoUrl
    ) throws IOException {
        ArrayNode seed = cloneWithNullFields(removals);
        String[] gaParts = parseGa(ga);
        if (gaParts == null) {
            return seed;
        }

        ResolverContext resolverContext;
        try {
            resolverContext = newResolverContext(repoUrl);
        } catch (RuntimeException ex) {
            return seed;
        }

        List<String> versionsDescending;
        try {
            versionsDescending = listVersionsDescending(
                resolverContext,
                gaParts[0],
                gaParts[1],
                fromVersion,
                toVersion
            );
        } catch (Exception ex) {
            return seed;
        }

        if (versionsDescending.isEmpty()) {
            versionsDescending = new ArrayList<String>();
        }
        if (!versionsDescending.contains(fromVersion)) {
            versionsDescending.add(fromVersion);
        }

        Map<String, Path> sourcesCache = new HashMap<String, Path>();
        return enrichRemovals(
            removals,
            versionsDescending,
            version -> {
                if (sourcesCache.containsKey(version)) {
                    return sourcesCache.get(version);
                }
                Path resolved = resolveSourcesJar(
                    resolverContext,
                    gaParts[0],
                    gaParts[1],
                    version
                );
                sourcesCache.put(version, resolved);
                return resolved;
            }
        );
    }

    static ArrayNode enrichRemovals(
        ArrayNode removals,
        List<String> versionsDescending,
        SourceJarProvider sourceJarProvider
    ) throws IOException {
        ArrayNode enriched = cloneWithNullFields(removals);
        Set<Integer> unresolved = new LinkedHashSet<Integer>();
        for (int i = 0; i < enriched.size(); i++) {
            unresolved.add(i);
        }

        for (String version : versionsDescending) {
            if (unresolved.isEmpty()) {
                break;
            }

            Path sourcesJar;
            try {
                sourcesJar = sourceJarProvider.resolve(version);
            } catch (Exception ex) {
                continue;
            }
            if (sourcesJar == null || !Files.isRegularFile(sourcesJar)) {
                continue;
            }

            SourceDeprecatedIndex index = SourceDeprecatedIndex.fromSourcesJar(
                sourcesJar,
                enriched,
                unresolved
            );
            if (index.isEmpty()) {
                continue;
            }

            List<Integer> resolvedNow = new ArrayList<Integer>();
            for (Integer idx : unresolved) {
                JsonNode node = enriched.get(idx.intValue());
                if (!(node instanceof ObjectNode)) {
                    continue;
                }
                ObjectNode removal = (ObjectNode) node;
                String note = index.findDeprecatedNote(removal);
                if (note == null || note.isEmpty()) {
                    continue;
                }
                removal.put("javadoc_last_ver", version);
                removal.put("javadoc_last_note", note);
                resolvedNow.add(idx);
            }
            unresolved.removeAll(resolvedNow);
        }

        return enriched;
    }

    private static ArrayNode readArray(Path file) throws IOException {
        JsonNode root = JSON.readTree(file.toFile());
        if (!(root instanceof ArrayNode)) {
            throw new IOException(
                "Expected JSON array in removals file: " + String.valueOf(file)
            );
        }
        return (ArrayNode) root;
    }

    private static void writeArray(Path file, ArrayNode values) throws IOException {
        Path parent = file.getParent();
        if (parent != null) {
            Files.createDirectories(parent);
        }
        JSON.writerWithDefaultPrettyPrinter().writeValue(file.toFile(), values);
    }

    private static ArrayNode cloneWithNullFields(ArrayNode removals) {
        ArrayNode enriched = JSON.createArrayNode();
        for (JsonNode node : removals) {
            ObjectNode copy;
            if (node instanceof ObjectNode) {
                copy = ((ObjectNode) node).deepCopy();
            } else {
                copy = JSON.createObjectNode();
            }
            copy.putNull("javadoc_last_ver");
            copy.putNull("javadoc_last_note");
            enriched.add(copy);
        }
        return enriched;
    }

    private static List<String> listVersionsDescending(
        ResolverContext resolverContext,
        String groupId,
        String artifactId,
        String fromVersion,
        String toVersion
    ) throws Exception {
        String range = "[" + fromVersion + "," + toVersion + ")";
        Artifact artifact = new DefaultArtifact(
            groupId + ":" + artifactId + ":" + range
        );
        VersionRangeRequest request = new VersionRangeRequest();
        request.setArtifact(artifact);
        request.setRepositories(resolverContext.repositories);
        VersionRangeResult result = resolverContext.system.resolveVersionRange(
            resolverContext.session,
            request
        );

        List<String> versions = new ArrayList<String>();
        for (Version version : result.getVersions()) {
            versions.add(version.toString());
        }
        Collections.sort(
            versions,
            (left, right) ->
                new ComparableVersion(right).compareTo(
                        new ComparableVersion(left)
                    )
        );
        return versions;
    }

    private static Path resolveSourcesJar(
        ResolverContext resolverContext,
        String groupId,
        String artifactId,
        String version
    ) {
        Artifact sourceArtifact = new DefaultArtifact(
            groupId,
            artifactId,
            "sources",
            "jar",
            version
        );
        ArtifactRequest request = new ArtifactRequest();
        request.setArtifact(sourceArtifact);
        request.setRepositories(resolverContext.repositories);
        try {
            ArtifactResult result = resolverContext.system.resolveArtifact(
                resolverContext.session,
                request
            );
            File file = result.getArtifact().getFile();
            if (file == null || !file.isFile()) {
                return null;
            }
            return file.toPath();
        } catch (Exception ignored) {
            return null;
        }
    }

    private static ResolverContext newResolverContext(String repoUrl) {
        boolean useSettingsMirrors = trimToNull(repoUrl) == null;
        Document settings = readSettingsDocument();
        String effectiveRepoUrl =
            useSettingsMirrors
                ? resolveRepoUrlFromSettings(
                    settings,
                    CENTRAL_ID,
                    "default",
                    DEFAULT_REPO_URL
                )
                : repoUrl;

        RepositorySystem system = newRepositorySystem();
        RepositorySystemSession session = newSession(
            system,
            settings,
            useSettingsMirrors
        );
        List<RemoteRepository> repositories = Collections.singletonList(
            new RemoteRepository.Builder(
                CENTRAL_ID,
                "default",
                effectiveRepoUrl
            ).build()
        );
        return new ResolverContext(system, session, repositories);
    }

    private static RepositorySystem newRepositorySystem() {
        DefaultServiceLocator locator =
            MavenRepositorySystemUtils.newServiceLocator();
        locator.addService(
            RepositoryConnectorFactory.class,
            BasicRepositoryConnectorFactory.class
        );
        locator.addService(
            TransporterFactory.class,
            FileTransporterFactory.class
        );
        locator.addService(
            TransporterFactory.class,
            HttpTransporterFactory.class
        );
        return locator.getService(RepositorySystem.class);
    }

    private static RepositorySystemSession newSession(
        RepositorySystem system,
        Document settings,
        boolean useSettingsMirrors
    ) {
        DefaultRepositorySystemSession session =
            MavenRepositorySystemUtils.newSession();

        String localRepoPath = System.getProperty("maven.repo.local");
        if (isBlank(localRepoPath)) {
            localRepoPath =
                System.getProperty("user.home") + "/.m2/repository";
        }
        session.setLocalRepositoryManager(
            system.newLocalRepositoryManager(
                session,
                new LocalRepository(localRepoPath)
            )
        );

        if (settings != null) {
            applySettingsAuthentication(session, settings);
            applySettingsProxy(session, settings);
            if (useSettingsMirrors) {
                applySettingsMirrors(session, settings);
                session.setIgnoreArtifactDescriptorRepositories(true);
            }
        }
        return session;
    }

    private static void applySettingsMirrors(
        DefaultRepositorySystemSession session,
        Document settings
    ) {
        DefaultMirrorSelector selector = buildMirrorSelector(settings);
        if (selector != null) {
            session.setMirrorSelector(selector);
        }
    }

    private static String resolveRepoUrlFromSettings(
        Document settings,
        String repoId,
        String repoType,
        String defaultRepoUrl
    ) {
        DefaultMirrorSelector selector = buildMirrorSelector(settings);
        if (selector == null) {
            return defaultRepoUrl;
        }
        RemoteRepository mirror = selector.getMirror(
            new RemoteRepository.Builder(repoId, repoType, defaultRepoUrl).build()
        );
        if (mirror == null || isBlank(mirror.getUrl())) {
            return defaultRepoUrl;
        }
        return mirror.getUrl();
    }

    private static DefaultMirrorSelector buildMirrorSelector(Document settings) {
        if (settings == null) {
            return null;
        }
        org.w3c.dom.NodeList mirrors = settings.getElementsByTagName("mirror");
        DefaultMirrorSelector selector = new DefaultMirrorSelector();
        boolean added = false;
        for (int i = 0; i < mirrors.getLength(); i++) {
            if (!(mirrors.item(i) instanceof Element)) {
                continue;
            }
            Element mirror = (Element) mirrors.item(i);
            String id = trimToNull(childText(mirror, "id"));
            String url = trimToNull(childText(mirror, "url"));
            String mirrorOf = trimToNull(childText(mirror, "mirrorOf"));
            if (id == null || url == null || mirrorOf == null) {
                continue;
            }
            String mirrorOfLayouts = trimToNull(
                childText(mirror, "mirrorOfLayouts")
            );
            if (mirrorOfLayouts == null) {
                mirrorOfLayouts = "*";
            }
            selector.add(id, url, "default", false, mirrorOf, mirrorOfLayouts);
            added = true;
        }
        return added ? selector : null;
    }

    private static void applySettingsProxy(
        DefaultRepositorySystemSession session,
        Document settings
    ) {
        org.w3c.dom.NodeList proxies = settings.getElementsByTagName("proxy");
        DefaultProxySelector selector = new DefaultProxySelector();
        boolean added = false;
        for (int i = 0; i < proxies.getLength(); i++) {
            if (!(proxies.item(i) instanceof Element)) {
                continue;
            }
            Element proxy = (Element) proxies.item(i);
            if (!isProxyActive(proxy)) {
                continue;
            }

            String host = trimToNull(childText(proxy, "host"));
            if (host == null) {
                continue;
            }
            String protocol = trimToNull(childText(proxy, "protocol"));
            if (protocol == null) {
                protocol = Proxy.TYPE_HTTPS;
            }
            protocol = protocol.toLowerCase(Locale.ROOT);
            if (
                !Proxy.TYPE_HTTP.equals(protocol) &&
                !Proxy.TYPE_HTTPS.equals(protocol)
            ) {
                protocol = Proxy.TYPE_HTTPS;
            }
            int port = parsePort(childText(proxy, "port"));
            if (port <= 0) {
                port = Proxy.TYPE_HTTP.equals(protocol) ? 80 : 443;
            }
            Authentication auth = buildAuthentication(
                trimToNull(childText(proxy, "username")),
                trimToNull(childText(proxy, "password")),
                null,
                null
            );
            selector.add(
                new Proxy(protocol, host, port, auth),
                trimToNull(childText(proxy, "nonProxyHosts"))
            );
            added = true;
        }
        if (added) {
            session.setProxySelector(selector);
        }
    }

    private static void applySettingsAuthentication(
        DefaultRepositorySystemSession session,
        Document settings
    ) {
        org.w3c.dom.NodeList servers = settings.getElementsByTagName("server");
        DefaultAuthenticationSelector selector =
            new DefaultAuthenticationSelector();
        boolean added = false;
        for (int i = 0; i < servers.getLength(); i++) {
            if (!(servers.item(i) instanceof Element)) {
                continue;
            }
            Element server = (Element) servers.item(i);
            String id = trimToNull(childText(server, "id"));
            if (id == null) {
                continue;
            }
            Authentication auth = buildAuthentication(
                trimToNull(childText(server, "username")),
                trimToNull(childText(server, "password")),
                trimToNull(childText(server, "privateKey")),
                trimToNull(childText(server, "passphrase"))
            );
            if (auth == null) {
                continue;
            }
            selector.add(id, auth);
            added = true;
        }
        if (added) {
            session.setAuthenticationSelector(selector);
        }
    }

    private static Authentication buildAuthentication(
        String username,
        String password,
        String privateKey,
        String passphrase
    ) {
        if (
            username == null &&
            password == null &&
            privateKey == null &&
            passphrase == null
        ) {
            return null;
        }
        AuthenticationBuilder builder = new AuthenticationBuilder();
        if (username != null) {
            builder.addUsername(username);
        }
        if (password != null) {
            builder.addPassword(password);
        }
        if (privateKey != null) {
            builder.addPrivateKey(privateKey, passphrase);
        }
        return builder.build();
    }

    private static boolean isProxyActive(Element proxy) {
        String active = trimToNull(childText(proxy, "active"));
        return active == null || !"false".equalsIgnoreCase(active);
    }

    private static int parsePort(String portText) {
        String trimmed = trimToNull(portText);
        if (trimmed == null) {
            return -1;
        }
        try {
            return Integer.parseInt(trimmed);
        } catch (NumberFormatException ignored) {
            return -1;
        }
    }

    private static Document readSettingsDocument() {
        File settingsFile = new File(
            System.getProperty("user.home"),
            ".m2/settings.xml"
        );
        if (!settingsFile.isFile()) {
            return null;
        }
        try {
            DocumentBuilderFactory factory = DocumentBuilderFactory.newInstance();
            factory.setNamespaceAware(false);
            return factory.newDocumentBuilder().parse(settingsFile);
        } catch (Exception ignored) {
            return null;
        }
    }

    private static String childText(Element parent, String tagName) {
        org.w3c.dom.NodeList nodes = parent.getElementsByTagName(tagName);
        if (nodes.getLength() == 0 || nodes.item(0) == null) {
            return null;
        }
        return nodes.item(0).getTextContent();
    }

    private static String[] parseGa(String ga) {
        if (ga == null) {
            return null;
        }
        int separator = ga.indexOf(':');
        if (separator <= 0 || separator + 1 >= ga.length()) {
            return null;
        }
        return new String[] { ga.substring(0, separator), ga.substring(separator + 1) };
    }

    private static String trimToNull(String value) {
        if (value == null) {
            return null;
        }
        String trimmed = value.trim();
        return trimmed.isEmpty() ? null : trimmed;
    }

    private static boolean isBlank(String value) {
        return trimToNull(value) == null;
    }

    private static void printUsage() {
        System.err.println(
            "Usage: JapicmpRemovalJavadocEnricherCli " +
            "--ga <group:artifact> --from <version> --to <version> " +
            "--removals-file <path> --output <path> [--repo-url <url>]"
        );
    }

    interface SourceJarProvider {
        Path resolve(String version) throws Exception;
    }

    private static final class SourceDeprecatedIndex {
        private final Map<String, String> classNotes =
            new HashMap<String, String>();
        private final Map<String, Map<String, List<SignatureNote>>> methodNotes =
            new HashMap<String, Map<String, List<SignatureNote>>>();
        private final Map<String, List<SignatureNote>> constructorNotes =
            new HashMap<String, List<SignatureNote>>();
        private final Map<String, Map<String, String>> fieldNotes =
            new HashMap<String, Map<String, String>>();

        static SourceDeprecatedIndex fromSourcesJar(
            Path sourcesJar,
            ArrayNode removals,
            Set<Integer> unresolved
        ) throws IOException {
            SourceDeprecatedIndex index = new SourceDeprecatedIndex();
            ParserConfiguration parserConfiguration = new ParserConfiguration();
            parserConfiguration.setLanguageLevel(
                ParserConfiguration.LanguageLevel.BLEEDING_EDGE
            );
            JavaParser parser = new JavaParser(parserConfiguration);

            try (JarFile jarFile = new JarFile(sourcesJar.toFile())) {
                Set<String> availableJavaEntries = new HashSet<String>();
                Enumeration<JarEntry> entries = jarFile.entries();
                while (entries.hasMoreElements()) {
                    JarEntry entry = entries.nextElement();
                    if (!entry.isDirectory() && entry.getName().endsWith(".java")) {
                        availableJavaEntries.add(entry.getName());
                    }
                }

                Set<String> selectedEntries = selectSourceEntries(
                    removals,
                    unresolved,
                    availableJavaEntries
                );
                for (String entryName : selectedEntries) {
                    JarEntry entry = jarFile.getJarEntry(entryName);
                    if (entry == null) {
                        continue;
                    }
                    CompilationUnit compilationUnit = parseCompilationUnit(
                        parser,
                        jarFile,
                        entry
                    );
                    if (compilationUnit == null) {
                        continue;
                    }
                    indexCompilationUnit(compilationUnit, index);
                }
            }

            return index;
        }

        boolean isEmpty() {
            return (
                classNotes.isEmpty() &&
                methodNotes.isEmpty() &&
                constructorNotes.isEmpty() &&
                fieldNotes.isEmpty()
            );
        }

        String findDeprecatedNote(ObjectNode removal) {
            String kind = textValue(removal, "kind");
            String className = normalizeClassName(textValue(removal, "class"));
            String member = textValue(removal, "member");
            if ("method".equals(kind)) {
                return findMethodNote(className, member);
            }
            if ("constructor".equals(kind)) {
                return findConstructorNote(className, member);
            }
            if ("field".equals(kind)) {
                return findFieldNote(className, member);
            }
            if ("interface".equals(kind) || "superclass".equals(kind)) {
                if (looksLikeTypeName(member)) {
                    String memberType = normalizeClassName(member);
                    String memberNote = classNotes.get(memberType);
                    if (memberNote != null) {
                        return memberNote;
                    }
                }
                return classNotes.get(className);
            }
            if ("class".equals(kind)) {
                String note = classNotes.get(className);
                if (note != null) {
                    return note;
                }
                if (looksLikeTypeName(member)) {
                    return classNotes.get(normalizeClassName(member));
                }
                return null;
            }
            return null;
        }

        private String findMethodNote(String className, String member) {
            MemberSignature signature = MemberSignature.parse(member);
            if (!signature.isValid()) {
                return null;
            }
            Map<String, List<SignatureNote>> classMethods = methodNotes.get(
                className
            );
            if (classMethods == null) {
                return null;
            }
            List<SignatureNote> candidates = classMethods.get(signature.getName());
            if (candidates == null) {
                return null;
            }
            for (SignatureNote candidate : candidates) {
                if (
                    parametersEquivalent(
                        signature.getParameters(),
                        candidate.getParameters()
                    )
                ) {
                    return candidate.getNote();
                }
            }
            return null;
        }

        private String findConstructorNote(String className, String member) {
            MemberSignature signature = MemberSignature.parse(member);
            if (!signature.isValid()) {
                return null;
            }
            List<SignatureNote> candidates = constructorNotes.get(className);
            if (candidates == null) {
                return null;
            }
            for (SignatureNote candidate : candidates) {
                if (
                    parametersEquivalent(
                        signature.getParameters(),
                        candidate.getParameters()
                    )
                ) {
                    return candidate.getNote();
                }
            }
            return null;
        }

        private String findFieldNote(String className, String member) {
            if (member == null || member.isEmpty()) {
                return null;
            }
            Map<String, String> classFieldNotes = fieldNotes.get(className);
            if (classFieldNotes == null) {
                return null;
            }
            return classFieldNotes.get(member);
        }
    }

    private static CompilationUnit parseCompilationUnit(
        JavaParser parser,
        JarFile jarFile,
        JarEntry entry
    ) {
        try (InputStream input = jarFile.getInputStream(entry)) {
            String source = new String(input.readAllBytes(), StandardCharsets.UTF_8);
            ParseResult<CompilationUnit> result = parser.parse(source);
            if (!result.getResult().isPresent()) {
                return null;
            }
            return result.getResult().get();
        } catch (IOException ignored) {
            return null;
        }
    }

    private static Set<String> selectSourceEntries(
        ArrayNode removals,
        Set<Integer> unresolved,
        Set<String> availableEntries
    ) {
        Set<String> selected = new LinkedHashSet<String>();
        for (Integer idx : unresolved) {
            JsonNode node = removals.get(idx.intValue());
            if (!(node instanceof ObjectNode)) {
                continue;
            }
            ObjectNode removal = (ObjectNode) node;
            addSourceEntryCandidate(
                selected,
                availableEntries,
                textValue(removal, "class")
            );

            String kind = textValue(removal, "kind");
            if ("interface".equals(kind) || "superclass".equals(kind)) {
                addSourceEntryCandidate(
                    selected,
                    availableEntries,
                    textValue(removal, "member")
                );
            }
        }
        return selected;
    }

    private static void addSourceEntryCandidate(
        Set<String> selectedEntries,
        Set<String> availableEntries,
        String className
    ) {
        if (!looksLikeTypeName(className)) {
            return;
        }
        String path = normalizeClassName(className).replace('.', '/');
        while (path != null && !path.isEmpty()) {
            String candidate = path + ".java";
            if (availableEntries.contains(candidate)) {
                selectedEntries.add(candidate);
                return;
            }
            int cut = path.lastIndexOf('/');
            if (cut <= 0) {
                return;
            }
            path = path.substring(0, cut);
        }
    }

    private static void indexCompilationUnit(
        CompilationUnit compilationUnit,
        SourceDeprecatedIndex index
    ) {
        TypeNameContext typeNameContext = TypeNameContext.fromCompilationUnit(
            compilationUnit
        );
        String packageName =
            compilationUnit
                .getPackageDeclaration()
                .map(pkg -> pkg.getNameAsString())
                .orElse("");
        for (TypeDeclaration<?> type : compilationUnit.getTypes()) {
            indexTypeDeclaration(type, packageName, null, typeNameContext, index);
        }
    }

    private static void indexTypeDeclaration(
        TypeDeclaration<?> type,
        String packageName,
        String outerClassName,
        TypeNameContext typeNameContext,
        SourceDeprecatedIndex index
    ) {
        String className =
            outerClassName == null
                ? qualify(packageName, type.getNameAsString())
                : outerClassName + "." + type.getNameAsString();
        className = normalizeClassName(className);

        String classNote = deprecatedNote(type);
        if (classNote != null) {
            index.classNotes.put(className, classNote);
        }

        if (!type.getMembers().isEmpty()) {
            for (BodyDeclaration<?> member : type.getMembers()) {
                if (member instanceof MethodDeclaration) {
                    MethodDeclaration method = (MethodDeclaration) member;
                    String note = deprecatedNote(method);
                    if (note == null) {
                        continue;
                    }
                    List<String> parameterTypes = parameterTypes(
                        method.getParameters(),
                        typeNameContext
                    );
                    Map<String, List<SignatureNote>> byName = index.methodNotes.get(
                        className
                    );
                    if (byName == null) {
                        byName = new HashMap<String, List<SignatureNote>>();
                        index.methodNotes.put(className, byName);
                    }
                    List<SignatureNote> signatures = byName.get(
                        method.getNameAsString()
                    );
                    if (signatures == null) {
                        signatures = new ArrayList<SignatureNote>();
                        byName.put(method.getNameAsString(), signatures);
                    }
                    signatures.add(new SignatureNote(parameterTypes, note));
                    continue;
                }

                if (member instanceof ConstructorDeclaration) {
                    ConstructorDeclaration constructor =
                        (ConstructorDeclaration) member;
                    String note = deprecatedNote(constructor);
                    if (note == null) {
                        continue;
                    }
                    List<SignatureNote> signatures = index.constructorNotes.get(
                        className
                    );
                    if (signatures == null) {
                        signatures = new ArrayList<SignatureNote>();
                        index.constructorNotes.put(className, signatures);
                    }
                    signatures.add(
                        new SignatureNote(
                            parameterTypes(
                                constructor.getParameters(),
                                typeNameContext
                            ),
                            note
                        )
                    );
                    continue;
                }

                if (member instanceof FieldDeclaration) {
                    FieldDeclaration fieldDeclaration = (FieldDeclaration) member;
                    String note = deprecatedNote(fieldDeclaration);
                    if (note == null) {
                        continue;
                    }
                    Map<String, String> byName = index.fieldNotes.get(className);
                    if (byName == null) {
                        byName = new HashMap<String, String>();
                        index.fieldNotes.put(className, byName);
                    }
                    for (int variableIdx = 0; variableIdx < fieldDeclaration.getVariables().size(); variableIdx++) {
                        byName.put(
                            fieldDeclaration
                                .getVariables()
                                .get(variableIdx)
                                .getNameAsString(),
                            note
                        );
                    }
                    continue;
                }

                if (member instanceof TypeDeclaration<?>) {
                    indexTypeDeclaration(
                        (TypeDeclaration<?>) member,
                        packageName,
                        className,
                        typeNameContext,
                        index
                    );
                }
            }
        }
    }

    private static String qualify(String packageName, String simpleName) {
        if (packageName == null || packageName.isEmpty()) {
            return simpleName;
        }
        return packageName + "." + simpleName;
    }

    private static String deprecatedNote(NodeWithJavadoc<?> node) {
        Optional<JavadocComment> comment = node.getJavadocComment();
        if (!comment.isPresent()) {
            return null;
        }
        return extractDeprecatedTag(comment.get().getContent());
    }

    private static String extractDeprecatedTag(String content) {
        if (content == null || content.trim().isEmpty()) {
            return null;
        }
        String[] lines = content.replace("\r", "").split("\n");
        StringBuilder deprecated = new StringBuilder();
        boolean capture = false;
        for (String rawLine : lines) {
            String line = rawLine.trim();
            if (line.startsWith("*")) {
                line = line.substring(1).trim();
            }
            if (!capture) {
                int idx = line.indexOf("@deprecated");
                if (idx < 0) {
                    continue;
                }
                capture = true;
                String tail = line.substring(idx + "@deprecated".length()).trim();
                if (!tail.isEmpty()) {
                    if (deprecated.length() > 0) {
                        deprecated.append(' ');
                    }
                    deprecated.append(tail);
                }
                continue;
            }

            if (line.startsWith("@")) {
                break;
            }
            if (line.isEmpty()) {
                continue;
            }
            if (deprecated.length() > 0) {
                deprecated.append(' ');
            }
            deprecated.append(line);
        }

        String normalized = normalizeWhitespace(deprecated.toString());
        if (normalized.isEmpty()) {
            return null;
        }
        return normalized;
    }

    private static List<String> parameterTypes(
        NodeList<com.github.javaparser.ast.body.Parameter> parameters,
        TypeNameContext typeNameContext
    ) {
        List<String> types = new ArrayList<String>();
        for (com.github.javaparser.ast.body.Parameter parameter : parameters) {
            types.add(
                normalizeTypeName(
                    parameter.getType(),
                    parameter.isVarArgs(),
                    typeNameContext
                )
            );
        }
        return types;
    }

    private static String normalizeTypeName(
        Type type,
        boolean isVarArgs,
        TypeNameContext typeNameContext
    ) {
        int arrayDepth = arrayDepth(type) + (isVarArgs ? 1 : 0);
        String rawBase = typeBaseName(type);
        String base = eraseGenerics(rawBase);
        base = normalizeClassName(base);
        String resolvedBase = resolveTypeBase(base, typeNameContext);
        StringBuilder out = new StringBuilder(resolvedBase);
        for (int i = 0; i < arrayDepth; i++) {
            out.append("[]");
        }
        return out.toString();
    }

    private static int arrayDepth(Type type) {
        int depth = 0;
        Type current = type;
        while (current.isArrayType()) {
            depth++;
            current = current.asArrayType().getComponentType();
        }
        return depth;
    }

    private static String typeBaseName(Type type) {
        Type current = type;
        while (current.isArrayType()) {
            current = current.asArrayType().getComponentType();
        }
        if (current instanceof PrimitiveType) {
            return current.asString();
        }
        return current.asString();
    }

    private static String resolveTypeBase(
        String rawBase,
        TypeNameContext typeNameContext
    ) {
        String base = normalizeWhitespace(rawBase);
        if (base.isEmpty()) {
            return base;
        }
        if (isPrimitive(base)) {
            return base;
        }

        String firstSegment = firstSegment(base);
        String remainder = remainderAfterFirstSegment(base);

        if (typeNameContext.explicitImports.containsKey(base)) {
            return typeNameContext.explicitImports.get(base);
        }

        if (typeNameContext.explicitImports.containsKey(firstSegment)) {
            return typeNameContext.explicitImports.get(firstSegment) + remainder;
        }

        if (isLikelyQualifiedName(base)) {
            return base;
        }

        if (
            !base.contains(".") &&
            JAVA_LANG_TYPES.contains(firstSegment)
        ) {
            return "java.lang." + base;
        }

        if (
            !base.contains(".") &&
            !typeNameContext.wildcardImports.isEmpty()
        ) {
            return typeNameContext.wildcardImports.get(0) + "." + base;
        }

        if (typeNameContext.packageName != null && !typeNameContext.packageName.isEmpty()) {
            return typeNameContext.packageName + "." + base;
        }

        return base;
    }

    private static String firstSegment(String typeName) {
        int idx = typeName.indexOf('.');
        if (idx < 0) {
            return typeName;
        }
        return typeName.substring(0, idx);
    }

    private static String remainderAfterFirstSegment(String typeName) {
        int idx = typeName.indexOf('.');
        if (idx < 0) {
            return "";
        }
        return typeName.substring(idx);
    }

    private static boolean isPrimitive(String typeName) {
        return (
            "boolean".equals(typeName) ||
            "byte".equals(typeName) ||
            "char".equals(typeName) ||
            "double".equals(typeName) ||
            "float".equals(typeName) ||
            "int".equals(typeName) ||
            "long".equals(typeName) ||
            "short".equals(typeName) ||
            "void".equals(typeName)
        );
    }

    private static String eraseGenerics(String typeName) {
        if (typeName == null || typeName.isEmpty()) {
            return "";
        }
        StringBuilder out = new StringBuilder();
        int depth = 0;
        for (int i = 0; i < typeName.length(); i++) {
            char ch = typeName.charAt(i);
            if (ch == '<') {
                depth++;
                continue;
            }
            if (ch == '>') {
                if (depth > 0) {
                    depth--;
                }
                continue;
            }
            if (depth == 0) {
                out.append(ch);
            }
        }
        String erased = out.toString().trim();
        if (erased.startsWith("? extends ")) {
            erased = erased.substring("? extends ".length());
        } else if (erased.startsWith("? super ")) {
            erased = erased.substring("? super ".length());
        } else if ("?".equals(erased)) {
            erased = "java.lang.Object";
        }
        return erased.trim();
    }

    private static boolean parametersEquivalent(
        List<String> requested,
        List<String> candidate
    ) {
        if (requested.size() != candidate.size()) {
            return false;
        }
        for (int i = 0; i < requested.size(); i++) {
            String left = normalizeTypeForCompare(requested.get(i));
            String right = normalizeTypeForCompare(candidate.get(i));
            if (left.equals(right)) {
                continue;
            }
            if (!simpleTypeForCompare(left).equals(simpleTypeForCompare(right))) {
                return false;
            }
        }
        return true;
    }

    private static String normalizeTypeForCompare(String typeName) {
        String normalized =
            normalizeClassName(normalizeWhitespace(typeName)).replace("...", "[]");
        while (normalized.contains("[] []")) {
            normalized = normalized.replace("[] []", "[][]");
        }
        return normalized;
    }

    private static String simpleTypeForCompare(String typeName) {
        int arrays = 0;
        String base = typeName;
        while (base.endsWith("[]")) {
            arrays++;
            base = base.substring(0, base.length() - 2);
        }
        int lastDot = base.lastIndexOf('.');
        String simple = lastDot >= 0 ? base.substring(lastDot + 1) : base;
        StringBuilder out = new StringBuilder(simple);
        for (int i = 0; i < arrays; i++) {
            out.append("[]");
        }
        return out.toString();
    }

    private static String normalizeClassName(String className) {
        if (className == null) {
            return "";
        }
        return className.replace('$', '.').trim();
    }

    private static String normalizeWhitespace(String value) {
        if (value == null) {
            return "";
        }
        return value.trim().replaceAll("\\s+", " ");
    }

    private static String textValue(ObjectNode node, String key) {
        JsonNode value = node.get(key);
        if (value == null || value.isNull()) {
            return null;
        }
        String text = value.asText();
        return text == null || text.trim().isEmpty() ? null : text;
    }

    private static boolean looksLikeTypeName(String value) {
        if (value == null) {
            return false;
        }
        if (!value.contains(".")) {
            return false;
        }
        if (value.contains("(") || value.contains(")")) {
            return false;
        }
        return true;
    }

    private static boolean isLikelyQualifiedName(String value) {
        if (value == null || value.isEmpty()) {
            return false;
        }
        if (!value.contains(".")) {
            return false;
        }
        char first = value.charAt(0);
        return Character.isLowerCase(first);
    }

    private static final class TypeNameContext {
        private final String packageName;
        private final Map<String, String> explicitImports;
        private final List<String> wildcardImports;

        private TypeNameContext(
            String packageName,
            Map<String, String> explicitImports,
            List<String> wildcardImports
        ) {
            this.packageName = packageName;
            this.explicitImports = explicitImports;
            this.wildcardImports = wildcardImports;
        }

        static TypeNameContext fromCompilationUnit(CompilationUnit unit) {
            String packageName =
                unit
                    .getPackageDeclaration()
                    .map(pkg -> pkg.getNameAsString())
                    .orElse("");
            Map<String, String> imports = new HashMap<String, String>();
            List<String> wildcard = new ArrayList<String>();
            for (ImportDeclaration declaration : unit.getImports()) {
                if (declaration.isStatic()) {
                    continue;
                }
                String importName = declaration.getNameAsString();
                if (declaration.isAsterisk()) {
                    wildcard.add(importName);
                    continue;
                }
                imports.put(importName, importName);
                int dot = importName.lastIndexOf('.');
                if (dot >= 0 && dot + 1 < importName.length()) {
                    imports.put(importName.substring(dot + 1), importName);
                }
            }
            return new TypeNameContext(packageName, imports, wildcard);
        }
    }

    private static final class MemberSignature {
        private final String name;
        private final List<String> parameters;

        private MemberSignature(String name, List<String> parameters) {
            this.name = name;
            this.parameters = parameters;
        }

        static MemberSignature parse(String text) {
            if (text == null) {
                return new MemberSignature("", Collections.<String>emptyList());
            }
            String value = text.trim();
            int open = value.indexOf('(');
            int close = value.lastIndexOf(')');
            if (open < 0 || close < open) {
                return new MemberSignature(value, Collections.<String>emptyList());
            }
            String name = value.substring(0, open).trim();
            String params = value.substring(open + 1, close).trim();
            if (params.isEmpty()) {
                return new MemberSignature(name, Collections.<String>emptyList());
            }
            String[] parts = params.split(",");
            List<String> parameters = new ArrayList<String>();
            for (String part : parts) {
                parameters.add(normalizeTypeForCompare(part));
            }
            return new MemberSignature(name, parameters);
        }

        boolean isValid() {
            return name != null && !name.isEmpty();
        }

        String getName() {
            return name;
        }

        List<String> getParameters() {
            return parameters;
        }
    }

    private static final class SignatureNote {
        private final List<String> parameters;
        private final String note;

        private SignatureNote(List<String> parameters, String note) {
            this.parameters = parameters;
            this.note = note;
        }

        List<String> getParameters() {
            return parameters;
        }

        String getNote() {
            return note;
        }
    }

    private static final class ResolverContext {
        private final RepositorySystem system;
        private final RepositorySystemSession session;
        private final List<RemoteRepository> repositories;

        private ResolverContext(
            RepositorySystem system,
            RepositorySystemSession session,
            List<RemoteRepository> repositories
        ) {
            this.system = system;
            this.session = session;
            this.repositories = repositories;
        }
    }

    private static final class CliOptions {
        private final String ga;
        private final String fromVersion;
        private final String toVersion;
        private final Path removalsFile;
        private final Path outputFile;
        private final String repoUrl;

        private CliOptions(
            String ga,
            String fromVersion,
            String toVersion,
            Path removalsFile,
            Path outputFile,
            String repoUrl
        ) {
            this.ga = ga;
            this.fromVersion = fromVersion;
            this.toVersion = toVersion;
            this.removalsFile = removalsFile;
            this.outputFile = outputFile;
            this.repoUrl = repoUrl;
        }

        static CliOptions parse(String[] args) {
            String ga = null;
            String fromVersion = null;
            String toVersion = null;
            Path removalsFile = null;
            Path outputFile = null;
            String repoUrl = null;

            for (int i = 0; i < args.length; i++) {
                String arg = args[i];
                if ("--ga".equals(arg)) {
                    ga = readValue(args, ++i, arg);
                    continue;
                }
                if ("--from".equals(arg)) {
                    fromVersion = readValue(args, ++i, arg);
                    continue;
                }
                if ("--to".equals(arg)) {
                    toVersion = readValue(args, ++i, arg);
                    continue;
                }
                if ("--removals-file".equals(arg)) {
                    removalsFile = Path.of(readValue(args, ++i, arg));
                    continue;
                }
                if ("--output".equals(arg)) {
                    outputFile = Path.of(readValue(args, ++i, arg));
                    continue;
                }
                if ("--repo-url".equals(arg)) {
                    repoUrl = readValue(args, ++i, arg);
                    continue;
                }
                throw new IllegalArgumentException("Unknown argument: " + arg);
            }

            if (trimToNull(ga) == null) {
                throw new IllegalArgumentException("Missing required --ga");
            }
            if (trimToNull(fromVersion) == null) {
                throw new IllegalArgumentException("Missing required --from");
            }
            if (trimToNull(toVersion) == null) {
                throw new IllegalArgumentException("Missing required --to");
            }
            if (removalsFile == null) {
                throw new IllegalArgumentException(
                    "Missing required --removals-file"
                );
            }
            if (outputFile == null) {
                throw new IllegalArgumentException("Missing required --output");
            }

            return new CliOptions(
                ga,
                fromVersion,
                toVersion,
                removalsFile.toAbsolutePath().normalize(),
                outputFile.toAbsolutePath().normalize(),
                trimToNull(repoUrl)
            );
        }

        private static String readValue(String[] args, int index, String flag) {
            if (index >= args.length) {
                throw new IllegalArgumentException("Missing value for " + flag);
            }
            return args[index];
        }

        String getGa() {
            return ga;
        }

        String getFromVersion() {
            return fromVersion;
        }

        String getToVersion() {
            return toVersion;
        }

        Path getRemovalsFile() {
            return removalsFile;
        }

        Path getOutputFile() {
            return outputFile;
        }

        String getRepoUrl() {
            return repoUrl;
        }
    }
}
