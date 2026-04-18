import java.io.File;
import java.io.FileReader;
import java.io.Reader;
import java.util.Arrays;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Properties;
import java.util.Set;
import java.util.TreeMap;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import javax.xml.parsers.DocumentBuilderFactory;
import org.apache.maven.model.DependencyManagement;
import org.apache.maven.model.Model;
import org.apache.maven.model.Parent;
import org.apache.maven.model.io.xpp3.MavenXpp3Reader;
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
import org.eclipse.aether.spi.connector.RepositoryConnectorFactory;
import org.eclipse.aether.spi.connector.transport.TransporterFactory;
import org.eclipse.aether.transport.file.FileTransporterFactory;
import org.eclipse.aether.transport.http.HttpTransporterFactory;
import org.eclipse.aether.util.repository.AuthenticationBuilder;
import org.eclipse.aether.util.repository.DefaultAuthenticationSelector;
import org.eclipse.aether.util.repository.DefaultMirrorSelector;
import org.eclipse.aether.util.repository.DefaultProxySelector;
import org.w3c.dom.Document;
import org.w3c.dom.Element;
import org.w3c.dom.NodeList;

public class DependencyBomResolver {
    private static final String CENTRAL_ID = "central";
    private static final String DEFAULT_REPO_URL =
        "https://repo.maven.apache.org/maven2/";
    private static final Pattern PROPERTY_REF_PATTERN =
        Pattern.compile("\\$\\{([^}]+)\\}");

    public static void main(String[] args) throws Exception {
        if (args.length < 1 || args.length > 2) {
            printUsage();
            System.exit(1);
        }

        BomCoordinate bomCoordinate;
        try {
            bomCoordinate = parseBomCoordinate(args[0]);
        } catch (IllegalArgumentException ex) {
            System.err.println(ex.getMessage());
            printUsage();
            System.exit(1);
            return;
        }

        String repoUrl = args.length == 2 ? args[1] : DEFAULT_REPO_URL;
        boolean useSettingsMirrors = args.length != 2;
        Document settings = readSettingsDocument();
        if (useSettingsMirrors) {
            repoUrl = resolveRepoUrlFromSettings(
                settings,
                CENTRAL_ID,
                "default",
                repoUrl
            );
        }

        RepositorySystem repoSystem = newRepositorySystem();
        RepositorySystemSession session = newSession(
            repoSystem,
            settings,
            useSettingsMirrors
        );

        List<RemoteRepository> repos = Arrays.asList(
            new RemoteRepository.Builder(CENTRAL_ID, "default", repoUrl).build()
        );

        Artifact bom = new DefaultArtifact(
            bomCoordinate.groupId +
            ":" +
            bomCoordinate.artifactId +
            ":pom:" +
            bomCoordinate.version
        );

        Map<String, String> managed = new TreeMap<>();
        ResolveState state = new ResolveState();
        collectManagedDependencies(
            repoSystem,
            session,
            repos,
            bom,
            new HashMap<String, String>(),
            state,
            managed
        );

        System.out.println("{");
        System.out.println(
            "  \"bomCoordinate\": \"" + escape(bomCoordinate.toString()) + "\","
        );
        System.out.println("  \"dependencies\": [");

        boolean first = true;
        for (Map.Entry<String, String> e : managed.entrySet()) {
            if (!first) {
                System.out.println(",");
            }
            first = false;

            String[] parts = e.getKey().split(":");
            System.out.print("    {");
            System.out.print("\"groupId\":\"" + parts[0] + "\",");
            System.out.print("\"artifactId\":\"" + parts[1] + "\",");
            System.out.print("\"version\":\"" + e.getValue() + "\"");
            System.out.print("}");
        }

        System.out.println();
        System.out.println("  ]");
        System.out.println("}");
    }

    private static void printUsage() {
        System.err.println(
            "Usage: DependencyBomResolver <groupId:artifactId@version> [repo-url]"
        );
    }

    static String normalizeBomCoordinate(String raw) {
        return parseBomCoordinate(raw).toString();
    }

    private static BomCoordinate parseBomCoordinate(String raw) {
        String coordinate = trimToNull(raw);
        if (coordinate == null) {
            throw new IllegalArgumentException("Missing BOM coordinate");
        }

        int at = coordinate.indexOf('@');
        if (at <= 0 || at != coordinate.lastIndexOf('@')) {
            throw new IllegalArgumentException(
                "Invalid BOM coordinate format: " + coordinate
            );
        }

        String ga = coordinate.substring(0, at);
        String version = trimToNull(coordinate.substring(at + 1));
        if (version == null) {
            throw new IllegalArgumentException(
                "Missing version in BOM coordinate: " + coordinate
            );
        }

        int colon = ga.indexOf(':');
        if (colon <= 0 || colon != ga.lastIndexOf(':')) {
            throw new IllegalArgumentException(
                "Invalid groupId:artifactId section in BOM coordinate: " +
                coordinate
            );
        }
        String groupId = trimToNull(ga.substring(0, colon));
        String artifactId = trimToNull(ga.substring(colon + 1));
        if (groupId == null || artifactId == null) {
            throw new IllegalArgumentException(
                "Invalid groupId:artifactId section in BOM coordinate: " +
                coordinate
            );
        }

        return new BomCoordinate(groupId, artifactId, version);
    }

    private static String escape(String input) {
        if (input == null) {
            return "";
        }
        return input.replace("\\", "\\\\").replace("\"", "\\\"");
    }

    private static void collectManagedDependencies(
        RepositorySystem repoSystem,
        RepositorySystemSession session,
        List<RemoteRepository> repos,
        Artifact pomArtifact,
        Map<String, String> inheritedProperties,
        ResolveState state,
        Map<String, String> managed
    ) throws Exception {
        String artifactKey = artifactKey(pomArtifact);
        if (!state.collecting.add(artifactKey)) {
            return;
        }

        try {
            Model model = resolvePomModel(
                repoSystem,
                session,
                repos,
                pomArtifact,
                state
            );
            Map<String, String> properties = resolvePomProperties(
                repoSystem,
                session,
                repos,
                pomArtifact,
                inheritedProperties,
                state
            );

            DependencyManagement dependencyManagement =
                model.getDependencyManagement();
            if (dependencyManagement == null) {
                return;
            }

            List<org.apache.maven.model.Dependency> deps =
                dependencyManagement.getDependencies();
            if (deps == null) {
                return;
            }

            for (org.apache.maven.model.Dependency dep : deps) {
                String groupId = trimToNull(
                    resolvePlaceholders(dep.getGroupId(), properties)
                );
                String artifactId = trimToNull(
                    resolvePlaceholders(dep.getArtifactId(), properties)
                );
                String version = trimToNull(
                    resolvePlaceholders(dep.getVersion(), properties)
                );
                String type = trimToNull(
                    resolvePlaceholders(dep.getType(), properties)
                );
                String scope = trimToNull(
                    resolvePlaceholders(dep.getScope(), properties)
                );

                if (groupId == null || artifactId == null) {
                    continue;
                }
                if (type == null) {
                    type = "jar";
                }

                if ("import".equals(scope) && "pom".equals(type)) {
                    if (version == null) {
                        continue;
                    }
                    Artifact importedBom = new DefaultArtifact(
                        groupId + ":" + artifactId + ":pom:" + version
                    );
                    collectManagedDependencies(
                        repoSystem,
                        session,
                        repos,
                        importedBom,
                        properties,
                        state,
                        managed
                    );
                    continue;
                }

                if (version != null) {
                    managed.put(groupId + ":" + artifactId, version);
                }
            }
        } finally {
            state.collecting.remove(artifactKey);
        }
    }

    private static Model resolvePomModel(
        RepositorySystem repoSystem,
        RepositorySystemSession session,
        List<RemoteRepository> repos,
        Artifact pomArtifact,
        ResolveState state
    ) throws Exception {
        String artifactKey = artifactKey(pomArtifact);
        Model cached = state.modelCache.get(artifactKey);
        if (cached != null) {
            return cached;
        }

        ArtifactRequest request = new ArtifactRequest();
        request.setArtifact(pomArtifact);
        request.setRepositories(repos);
        ArtifactResult result = repoSystem.resolveArtifact(session, request);
        File pomFile = result.getArtifact().getFile();
        if (pomFile == null) {
            throw new IllegalStateException(
                "Resolved POM artifact has no file: " + pomArtifact
            );
        }

        Model model;
        try (Reader reader = new FileReader(pomFile)) {
            model = new MavenXpp3Reader().read(reader);
        }

        state.modelCache.put(artifactKey, model);
        return model;
    }

    private static Map<String, String> resolvePomProperties(
        RepositorySystem repoSystem,
        RepositorySystemSession session,
        List<RemoteRepository> repos,
        Artifact pomArtifact,
        Map<String, String> inheritedProperties,
        ResolveState state
    ) throws Exception {
        String artifactKey = artifactKey(pomArtifact);
        Map<String, String> cached = state.propertiesCache.get(artifactKey);
        if (cached != null) {
            return cached;
        }

        if (!state.resolvingProperties.add(artifactKey)) {
            return new HashMap<String, String>(inheritedProperties);
        }

        try {
            Model model = resolvePomModel(
                repoSystem,
                session,
                repos,
                pomArtifact,
                state
            );

            Map<String, String> properties = new HashMap<String, String>();
            if (inheritedProperties != null) {
                properties.putAll(inheritedProperties);
            }

            mergeRawProperties(properties, model.getProperties());

            Parent parent = model.getParent();
            if (parent != null) {
                String parentGroupId = trimToNull(
                    resolvePlaceholders(parent.getGroupId(), properties)
                );
                String parentArtifactId = trimToNull(
                    resolvePlaceholders(parent.getArtifactId(), properties)
                );
                String parentVersion = trimToNull(
                    resolvePlaceholders(parent.getVersion(), properties)
                );
                if (
                    parentGroupId != null &&
                    parentArtifactId != null &&
                    parentVersion != null
                ) {
                    Artifact parentPom = new DefaultArtifact(
                        parentGroupId +
                        ":" +
                        parentArtifactId +
                        ":pom:" +
                        parentVersion
                    );
                    Map<String, String> parentProperties = resolvePomProperties(
                        repoSystem,
                        session,
                        repos,
                        parentPom,
                        properties,
                        state
                    );
                    properties.putAll(parentProperties);
                }
            }

            String groupId = trimToNull(
                resolvePlaceholders(model.getGroupId(), properties)
            );
            String artifactId = trimToNull(
                resolvePlaceholders(model.getArtifactId(), properties)
            );
            String version = trimToNull(
                resolvePlaceholders(model.getVersion(), properties)
            );
            if (groupId == null && parent != null) {
                groupId = trimToNull(
                    resolvePlaceholders(parent.getGroupId(), properties)
                );
            }
            if (version == null && parent != null) {
                version = trimToNull(
                    resolvePlaceholders(parent.getVersion(), properties)
                );
            }

            setIfPresent(properties, "project.groupId", groupId);
            setIfPresent(properties, "project.artifactId", artifactId);
            setIfPresent(properties, "project.version", version);
            setIfPresent(properties, "pom.groupId", groupId);
            setIfPresent(properties, "pom.artifactId", artifactId);
            setIfPresent(properties, "pom.version", version);

            resolveAndMergeModelProperties(properties, model.getProperties());

            Map<String, String> immutable =
                new HashMap<String, String>(properties);
            state.propertiesCache.put(artifactKey, immutable);
            return immutable;
        } finally {
            state.resolvingProperties.remove(artifactKey);
        }
    }

    private static void mergeRawProperties(
        Map<String, String> target,
        Properties props
    ) {
        if (props == null) {
            return;
        }
        for (String name : props.stringPropertyNames()) {
            String value = props.getProperty(name);
            if (value != null) {
                target.put(name, value);
            }
        }
    }

    private static void resolveAndMergeModelProperties(
        Map<String, String> target,
        Properties props
    ) {
        if (props == null) {
            return;
        }
        for (int pass = 0; pass < 16; pass++) {
            boolean changed = false;
            for (String name : props.stringPropertyNames()) {
                String raw = props.getProperty(name);
                String resolved = resolvePlaceholders(raw, target);
                String previous = target.put(name, resolved);
                if (previous == null || !previous.equals(resolved)) {
                    changed = true;
                }
            }
            if (!changed) {
                break;
            }
        }
    }

    private static String resolvePlaceholders(
        String value,
        Map<String, String> properties
    ) {
        if (value == null) {
            return null;
        }

        String resolved = value;
        for (int pass = 0; pass < 24; pass++) {
            Matcher matcher = PROPERTY_REF_PATTERN.matcher(resolved);
            StringBuffer out = new StringBuffer();
            boolean replaced = false;

            while (matcher.find()) {
                String propertyName = matcher.group(1);
                String replacement = properties.get(propertyName);
                if (replacement == null) {
                    replacement = System.getProperty(propertyName);
                }
                if (replacement == null) {
                    replacement = matcher.group(0);
                } else {
                    replaced = true;
                }
                matcher.appendReplacement(out, Matcher.quoteReplacement(replacement));
            }
            matcher.appendTail(out);

            String next = out.toString();
            if (!replaced || next.equals(resolved)) {
                return next;
            }
            resolved = next;
        }

        return resolved;
    }

    private static String artifactKey(Artifact artifact) {
        return artifact.getGroupId() +
        ":" +
        artifact.getArtifactId() +
        ":" +
        artifact.getExtension() +
        ":" +
        artifact.getVersion();
    }

    private static void setIfPresent(
        Map<String, String> properties,
        String key,
        String value
    ) {
        if (value != null) {
            properties.put(key, value);
        }
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
        String repoUrl
    ) {
        DefaultMirrorSelector selector = buildMirrorSelector(settings);
        if (selector == null) {
            return repoUrl;
        }
        RemoteRepository mirror = selector.getMirror(
            new RemoteRepository.Builder(repoId, repoType, repoUrl).build()
        );
        if (mirror == null || isBlank(mirror.getUrl())) {
            return repoUrl;
        }
        return mirror.getUrl();
    }

    private static DefaultMirrorSelector buildMirrorSelector(Document settings) {
        if (settings == null) {
            return null;
        }
        NodeList mirrors = settings.getElementsByTagName("mirror");
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
        NodeList proxies = settings.getElementsByTagName("proxy");
        DefaultProxySelector selector = new DefaultProxySelector();
        boolean added = false;

        for (int i = 0; i < proxies.getLength(); i++) {
            if (!(proxies.item(i) instanceof Element)) {
                continue;
            }
            Element proxyEl = (Element) proxies.item(i);
            if (!isProxyActive(proxyEl)) {
                continue;
            }

            String host = trimToNull(childText(proxyEl, "host"));
            if (host == null) {
                continue;
            }

            String protocol = trimToNull(childText(proxyEl, "protocol"));
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

            int port = parsePort(childText(proxyEl, "port"));
            if (port <= 0) {
                port = Proxy.TYPE_HTTP.equals(protocol) ? 80 : 443;
            }

            Authentication auth = buildAuthentication(
                trimToNull(childText(proxyEl, "username")),
                trimToNull(childText(proxyEl, "password")),
                null,
                null
            );

            Proxy proxy = new Proxy(protocol, host, port, auth);
            selector.add(proxy, trimToNull(childText(proxyEl, "nonProxyHosts")));
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
        NodeList servers = settings.getElementsByTagName("server");
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
            if (auth != null) {
                selector.add(id, auth);
                added = true;
            }
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
        } catch (Exception e) {
            System.err.println(
                "Warning: failed to parse " +
                settingsFile.getAbsolutePath() +
                ": " +
                e.getMessage()
            );
            return null;
        }
    }

    private static String childText(Element parent, String tagName) {
        NodeList nodes = parent.getElementsByTagName(tagName);
        if (nodes.getLength() == 0 || nodes.item(0) == null) {
            return null;
        }
        return nodes.item(0).getTextContent();
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

    private static final class BomCoordinate {
        private final String groupId;
        private final String artifactId;
        private final String version;

        private BomCoordinate(String groupId, String artifactId, String version) {
            this.groupId = groupId;
            this.artifactId = artifactId;
            this.version = version;
        }

        @Override
        public String toString() {
            return groupId + ":" + artifactId + "@" + version;
        }
    }

    private static final class ResolveState {
        private final Map<String, Model> modelCache = new HashMap<String, Model>();
        private final Map<String, Map<String, String>> propertiesCache =
            new HashMap<String, Map<String, String>>();
        private final Set<String> collecting = new HashSet<String>();
        private final Set<String> resolvingProperties = new HashSet<String>();
    }
}
