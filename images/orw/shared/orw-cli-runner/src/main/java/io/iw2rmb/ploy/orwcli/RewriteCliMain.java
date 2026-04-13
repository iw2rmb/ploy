package io.iw2rmb.ploy.orwcli;

import org.eclipse.aether.DefaultRepositorySystemSession;
import org.eclipse.aether.RepositorySystem;
import org.eclipse.aether.artifact.DefaultArtifact;
import org.eclipse.aether.collection.CollectRequest;
import org.eclipse.aether.graph.Dependency;
import org.eclipse.aether.repository.LocalRepository;
import org.eclipse.aether.repository.RemoteRepository;
import org.eclipse.aether.resolution.ArtifactResult;
import org.eclipse.aether.resolution.DependencyRequest;
import org.eclipse.aether.resolution.DependencyResolutionException;
import org.eclipse.aether.resolution.DependencyResult;
import org.eclipse.aether.util.artifact.JavaScopes;
import org.eclipse.aether.util.filter.DependencyFilterUtils;
import org.eclipse.aether.util.repository.AuthenticationBuilder;
import org.eclipse.aether.supplier.RepositorySystemSupplier;
import org.apache.maven.repository.internal.MavenRepositorySystemUtils;
import org.openrewrite.ExecutionContext;
import org.openrewrite.InMemoryExecutionContext;
import org.openrewrite.Parser;
import org.openrewrite.Recipe;
import org.openrewrite.RecipeRun;
import org.openrewrite.Result;
import org.openrewrite.SourceFile;
import org.openrewrite.binary.Binary;
import org.openrewrite.config.Environment;
import org.openrewrite.config.YamlResourceLoader;
import org.openrewrite.internal.InMemoryLargeSourceSet;
import org.openrewrite.java.JavaParser;
import org.openrewrite.polyglot.OmniParser;

import java.io.IOException;
import java.io.InputStream;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.net.URL;
import java.net.URLClassLoader;
import java.security.CodeSource;
import java.nio.file.FileSystems;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.PathMatcher;
import java.nio.file.Paths;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.Deque;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Objects;
import java.util.Properties;
import java.util.Set;
import java.util.stream.Collectors;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

public final class RewriteCliMain {
    private static final String DEFAULT_REPO = "https://repo1.maven.org/maven2/";
    private static final String DEFAULT_REWRITE_VERSION = "8.74.3";
    private static final String BUILD_SYSTEM_GRADLE = "gradle";
    private static final String BUILD_SYSTEM_MAVEN = "maven";
    private static final String STACK_TOOL_ENV = "PLOY_STACK_TOOL";
    private static final String EXCLUDE_PATHS_ENV = "ORW_EXCLUDE_PATHS";
    private static final String GRADLE_PARSER_CLASS = "org.openrewrite.gradle.GradleParser";
    private static final String MAVEN_PARSER_CLASS = "org.openrewrite.maven.MavenParser";
    private static final Pattern REWRITE_CORE_JAR_PATTERN = Pattern.compile("^rewrite-core-(.+)\\.jar$");
    private static final Pattern VERSION_MAJOR_MINOR_PATTERN = Pattern.compile("^(\\d+)\\.(\\d+)(?:\\..*)?$");

    private RewriteCliMain() {
    }

    public static void main(String[] args) {
        try {
            CliOptions opts = CliOptions.parse(args);
            runBootstrap(opts, args);
        } catch (InputException e) {
            System.err.println("error: " + e.getMessage());
            System.exit(2);
        } catch (UnsupportedTypeAttributionException e) {
            System.err.println("type-attribution-unavailable: " + e.getMessage());
            System.exit(17);
        } catch (Throwable t) {
            if (isTypeAttributionIssue(t)) {
                System.err.println("type-attribution-unavailable: " + safeMessage(t));
                System.exit(17);
            }
            t.printStackTrace(System.err);
            System.exit(1);
        }
    }

    @SuppressWarnings("unused")
    public static void runIsolatedEntry(String[] args) throws Exception {
        CliOptions opts = CliOptions.parse(args);
        run(opts);
    }

    private static void runBootstrap(CliOptions opts, String[] rawArgs) throws Exception {
        Resolution resolution = resolveRecipeArtifacts(opts.coords, opts.repos, opts.repoUsername, opts.repoPassword);
        ensureRewriteCoreCompatibility(opts.coords, resolution.classpathJars);
        URL[] isolatedClasspath = buildIsolatedClasspath(resolution.classpathJars);
        try (IsolatedRewriteClassLoader isolated = new IsolatedRewriteClassLoader(
            isolatedClasspath,
            RewriteCliMain.class.getClassLoader()
        )) {
            Class<?> isolatedMain = Class.forName(RewriteCliMain.class.getName(), true, isolated);
            Method method = isolatedMain.getMethod("runIsolatedEntry", String[].class);
            method.invoke(null, (Object) rawArgs);
        } catch (InvocationTargetException e) {
            Throwable cause = e.getCause();
            if (cause instanceof Exception) {
                throw (Exception) cause;
            }
            if (cause instanceof Error) {
                throw (Error) cause;
            }
            throw new RuntimeException(cause);
        }
    }

    private static URL[] buildIsolatedClasspath(List<Path> resolvedJars) throws IOException {
        LinkedHashSet<URL> urls = new LinkedHashSet<>();
        CodeSource source = RewriteCliMain.class.getProtectionDomain().getCodeSource();
        if (source == null || source.getLocation() == null) {
            throw new IllegalStateException("Unable to determine runner code source location");
        }
        // Runner jar must be first to prevent resolved recipe dependencies from overriding runner APIs.
        urls.add(source.getLocation());
        for (Path jar : resolvedJars) {
            urls.add(jar.toUri().toURL());
        }
        return urls.toArray(new URL[0]);
    }

    private static void ensureRewriteCoreCompatibility(String coords, List<Path> classpathJars) {
        String runnerVersion = detectRunnerRewriteCoreVersion();
        String resolvedVersion = detectResolvedRewriteCoreVersion(classpathJars);
        if (resolvedVersion == null) {
            return;
        }
        if (isMajorMinorCompatible(runnerVersion, resolvedVersion)) {
            return;
        }
        throw new InputException(
            "Incompatible OpenRewrite runtime: runner rewrite-core " + runnerVersion
                + " is incompatible with resolved rewrite-core " + resolvedVersion
                + " for " + coords
                + ". Align RECIPE_VERSION with runner rewrite-core " + runnerVersion + "."
        );
    }

    private static String detectRunnerRewriteCoreVersion() {
        String fromProps = readRewriteCoreVersionFromPomProperties(RewriteCliMain.class.getClassLoader());
        if (fromProps != null) {
            return fromProps;
        }
        Package pkg = Environment.class.getPackage();
        if (pkg != null && pkg.getImplementationVersion() != null && !pkg.getImplementationVersion().isBlank()) {
            return pkg.getImplementationVersion().trim();
        }
        return DEFAULT_REWRITE_VERSION;
    }

    private static String readRewriteCoreVersionFromPomProperties(ClassLoader classLoader) {
        String resource = "META-INF/maven/org.openrewrite/rewrite-core/pom.properties";
        try (InputStream in = classLoader.getResourceAsStream(resource)) {
            if (in == null) {
                return null;
            }
            Properties properties = new Properties();
            properties.load(in);
            String version = properties.getProperty("version");
            if (version == null || version.isBlank()) {
                return null;
            }
            return version.trim();
        } catch (IOException e) {
            return null;
        }
    }

    private static String detectResolvedRewriteCoreVersion(List<Path> classpathJars) {
        for (Path jar : classpathJars) {
            String fileName = jar.getFileName() == null ? "" : jar.getFileName().toString();
            Matcher matcher = REWRITE_CORE_JAR_PATTERN.matcher(fileName);
            if (matcher.matches()) {
                return matcher.group(1);
            }
        }
        return null;
    }

    private static boolean isMajorMinorCompatible(String runnerVersion, String resolvedVersion) {
        String runnerMajorMinor = majorMinor(runnerVersion);
        String resolvedMajorMinor = majorMinor(resolvedVersion);
        if (runnerMajorMinor == null || resolvedMajorMinor == null) {
            return Objects.equals(runnerVersion, resolvedVersion);
        }
        return runnerMajorMinor.equals(resolvedMajorMinor);
    }

    private static String majorMinor(String version) {
        if (version == null) {
            return null;
        }
        Matcher matcher = VERSION_MAJOR_MINOR_PATTERN.matcher(version.trim());
        if (!matcher.matches()) {
            return null;
        }
        return matcher.group(1) + "." + matcher.group(2);
    }

    private static void run(CliOptions opts) throws Exception {
        Path workspace = opts.dir.toAbsolutePath().normalize();
        if (!Files.isDirectory(workspace)) {
            throw new InputException("--dir must point to an existing directory: " + workspace);
        }

        Resolution resolution = resolveRecipeArtifacts(opts.coords, opts.repos, opts.repoUsername, opts.repoPassword);
        ClassLoader recipeClassLoader = buildRecipeClassLoader(resolution.classpathJars);

        Recipe recipe = loadRecipe(opts, resolution, recipeClassLoader);
        ExecutionContext ctx = new InMemoryExecutionContext(t -> {
            throw new RuntimeException(t);
        });

        List<SourceFile> sourceFiles = parseWorkspace(workspace, ctx, opts.buildSystem, opts.excludePathMatchers);
        InMemoryLargeSourceSet before = new InMemoryLargeSourceSet(sourceFiles, recipeClassLoader);

        RecipeRun run = recipe.run(before, ctx);
        List<Result> results = run.getChangeset().getAllResults();

        applyResults(workspace, results);

        System.out.println("[rewrite] Parsed files: " + sourceFiles.size());
        System.out.println("[rewrite] Applied results: " + results.size());
    }

    private static Recipe loadRecipe(CliOptions opts, Resolution resolution, ClassLoader recipeClassLoader) throws IOException {
        Properties properties = new Properties();
        Environment.Builder builder = Environment.builder(properties);

        if (resolution.rootArtifact != null) {
            builder.scanJar(resolution.rootArtifact, resolution.dependencyJars, recipeClassLoader);
        }

        if (opts.config != null) {
            Path configPath = opts.config.toAbsolutePath().normalize();
            if (!Files.isRegularFile(configPath)) {
                throw new InputException("--config file does not exist: " + configPath);
            }
            try (InputStream is = Files.newInputStream(configPath)) {
                builder.load(new YamlResourceLoader(is, configPath.toUri(), properties));
            }
        }

        Recipe recipe = builder.build().activateRecipes(opts.recipes);
        if (!recipe.validate().isValid()) {
            throw new InputException("Recipe validation failed for active recipes: " + String.join(",", opts.recipes));
        }
        return recipe;
    }

    private static List<SourceFile> parseWorkspace(
        Path workspace,
        ExecutionContext ctx,
        String requestedBuildSystem,
        List<PathMatcher> excludePathMatchers
    ) {
        List<Parser> parsers = new ArrayList<>();
        parsers.add(newBuildSystemParser(requestedBuildSystem));
        parsers.add(JavaParser.fromJavaVersion().logCompilationWarningsAndErrors(true).build());
        parsers.addAll(OmniParser.defaultResourceParsers());

        OmniParser parser = OmniParser.builder(parsers).build();
        List<Path> accepted = parser.acceptedPaths(workspace);
        List<Path> filtered = filterAcceptedPaths(workspace, accepted, excludePathMatchers);
        if (filtered.size() != accepted.size()) {
            System.out.println("[rewrite] Excluded paths: " + (accepted.size() - filtered.size()));
        }
        return parser.parse(filtered, workspace, ctx).collect(Collectors.toList());
    }

    private static List<Path> filterAcceptedPaths(Path workspace, List<Path> accepted, List<PathMatcher> excludePathMatchers) {
        if (excludePathMatchers.isEmpty()) {
            return accepted;
        }
        return accepted.stream()
            .filter(path -> !matchesAnyExclude(workspace, path, excludePathMatchers))
            .collect(Collectors.toList());
    }

    private static boolean matchesAnyExclude(Path workspace, Path path, List<PathMatcher> excludePathMatchers) {
        Path relative = toWorkspaceRelativePath(workspace, path);
        Path fileName = relative.getFileName();
        String portable = relative.toString().replace('\\', '/');
        Path portablePath = portable.equals(relative.toString()) ? relative : Paths.get(portable);
        for (PathMatcher matcher : excludePathMatchers) {
            if (matcher.matches(relative)) {
                return true;
            }
            if (portablePath != relative && matcher.matches(portablePath)) {
                return true;
            }
            if (fileName != null && matcher.matches(fileName)) {
                return true;
            }
        }
        return false;
    }

    private static Path toWorkspaceRelativePath(Path workspace, Path path) {
        Path normalizedPath = path.toAbsolutePath().normalize();
        Path normalizedWorkspace = workspace.toAbsolutePath().normalize();
        if (normalizedPath.startsWith(normalizedWorkspace)) {
            return normalizedWorkspace.relativize(normalizedPath);
        }
        return path.normalize();
    }

    private static List<PathMatcher> compileExcludePathMatchers(String rawPatterns) {
        List<String> raw = CliOptions.splitCsv(rawPatterns);
        if (raw.isEmpty()) {
            return Collections.emptyList();
        }
        List<PathMatcher> matchers = new ArrayList<>(raw.size());
        for (String pattern : raw) {
            try {
                matchers.add(FileSystems.getDefault().getPathMatcher("glob:" + pattern));
            } catch (IllegalArgumentException e) {
                throw new InputException("invalid glob in " + EXCLUDE_PATHS_ENV + ": " + pattern);
            }
        }
        return Collections.unmodifiableList(matchers);
    }

    private static Parser newBuildSystemParser(String requestedBuildSystem) {
        String buildSystem = resolveBuildSystem(requestedBuildSystem);
        if (BUILD_SYSTEM_GRADLE.equals(buildSystem)) {
            return instantiateParser(GRADLE_PARSER_CLASS, BUILD_SYSTEM_GRADLE);
        }
        return instantiateParser(MAVEN_PARSER_CLASS, BUILD_SYSTEM_MAVEN);
    }

    private static String resolveBuildSystem(String requestedBuildSystem) {
        if (requestedBuildSystem != null && !requestedBuildSystem.isBlank()) {
            return normalizeBuildSystem(requestedBuildSystem, "--build-system");
        }
        String fromEnv = System.getenv(STACK_TOOL_ENV);
        if (fromEnv != null && !fromEnv.isBlank()) {
            return normalizeBuildSystem(fromEnv, STACK_TOOL_ENV);
        }

        boolean hasGradleParser = classAvailable(GRADLE_PARSER_CLASS);
        boolean hasMavenParser = classAvailable(MAVEN_PARSER_CLASS);

        if (hasGradleParser && !hasMavenParser) {
            return BUILD_SYSTEM_GRADLE;
        }
        if (hasMavenParser && !hasGradleParser) {
            return BUILD_SYSTEM_MAVEN;
        }
        if (hasGradleParser) {
            throw new InputException(
                "Both Gradle and Maven parsers are available; set --build-system gradle|maven or PLOY_STACK_TOOL"
            );
        }
        throw new InputException(
            "No build parser is available; include rewrite-gradle or rewrite-maven dependency"
        );
    }

    private static String normalizeBuildSystem(String raw, String source) {
        String normalized = raw.trim().toLowerCase(Locale.ROOT);
        if (BUILD_SYSTEM_GRADLE.equals(normalized) || BUILD_SYSTEM_MAVEN.equals(normalized)) {
            return normalized;
        }
        throw new InputException(source + " must be 'gradle' or 'maven'");
    }

    private static boolean classAvailable(String className) {
        try {
            Class.forName(className);
            return true;
        } catch (ClassNotFoundException e) {
            return false;
        }
    }

    private static Parser instantiateParser(String className, String buildSystem) {
        try {
            Class<?> parserClass = Class.forName(className);
            Method builderMethod = parserClass.getMethod("builder");
            Object builder = builderMethod.invoke(null);
            Method buildMethod = builder.getClass().getMethod("build");
            Object parser = buildMethod.invoke(builder);
            if (parser instanceof Parser) {
                return (Parser) parser;
            }
            throw new InputException("Parser for build system '" + buildSystem + "' is invalid: " + className);
        } catch (ClassNotFoundException e) {
            throw new InputException("Parser class not found for build system '" + buildSystem + "': " + className);
        } catch (NoSuchMethodException | IllegalAccessException | InvocationTargetException e) {
            throw new IllegalStateException("Failed to construct parser for build system '" + buildSystem + "'", e);
        }
    }

    private static void applyResults(Path workspace, List<Result> results) throws IOException {
        for (Result result : results) {
            SourceFile before = result.getBefore();
            SourceFile after = result.getAfter();

            if (after == null && before != null) {
                Path oldPath = resolveUnderWorkspace(workspace, before.getSourcePath());
                Files.deleteIfExists(oldPath);
                continue;
            }

            if (after != null) {
                Path newPath = resolveUnderWorkspace(workspace, after.getSourcePath());
                Files.createDirectories(newPath.getParent());
                byte[] content = after instanceof Binary ? ((Binary) after).getBytes() : after.printAllAsBytes();
                Files.write(newPath, content);

                if (after.getFileAttributes() != null) {
                    newPath.toFile().setReadable(after.getFileAttributes().isReadable(), false);
                    newPath.toFile().setWritable(after.getFileAttributes().isWritable(), false);
                    newPath.toFile().setExecutable(after.getFileAttributes().isExecutable(), false);
                }

                if (before != null) {
                    Path oldPath = resolveUnderWorkspace(workspace, before.getSourcePath());
                    if (!oldPath.equals(newPath)) {
                        Files.deleteIfExists(oldPath);
                    }
                }
            }
        }
    }

    private static Path resolveUnderWorkspace(Path workspace, Path relativePath) {
        Path resolved = workspace.resolve(relativePath).normalize();
        if (!resolved.startsWith(workspace)) {
            throw new InputException("Refusing to write outside workspace: " + relativePath);
        }
        return resolved;
    }

    private static ClassLoader buildRecipeClassLoader(List<Path> jars) throws IOException {
        List<URL> urls = new ArrayList<>(jars.size());
        for (Path jar : jars) {
            urls.add(jar.toUri().toURL());
        }
        return new URLClassLoader(urls.toArray(new URL[0]), RewriteCliMain.class.getClassLoader());
    }

    private static Resolution resolveRecipeArtifacts(
        String coords,
        List<String> repos,
        String repoUsername,
        String repoPassword
    ) throws DependencyResolutionException {
        RepositorySystem repoSystem = newRepositorySystem();
        DefaultRepositorySystemSession session = MavenRepositorySystemUtils.newSession();
        session.setSystemProperties(System.getProperties());
        session.setUserProperties(new Properties());
        Path localM2 = Paths.get(System.getProperty("user.home"), ".m2", "repository");
        session.setLocalRepositoryManager(repoSystem.newLocalRepositoryManager(session, new LocalRepository(localM2.toFile())));

        List<RemoteRepository> remoteRepositories = new ArrayList<>();
        int i = 0;
        for (String repo : repos) {
            String normalized = repo.endsWith("/") ? repo : repo + "/";
            RemoteRepository.Builder builder = new RemoteRepository.Builder("repo-" + (++i), "default", normalized);
            if (repoUsername != null && repoPassword != null) {
                builder.setAuthentication(new AuthenticationBuilder().addUsername(repoUsername).addPassword(repoPassword).build());
            }
            remoteRepositories.add(builder.build());
        }
        if (remoteRepositories.isEmpty()) {
            remoteRepositories.add(new RemoteRepository.Builder("central", "default", DEFAULT_REPO).build());
        }

        DefaultArtifact rootArtifact = new DefaultArtifact(coords);
        Dependency rootDependency = new Dependency(rootArtifact, JavaScopes.RUNTIME);

        CollectRequest collectRequest = new CollectRequest();
        collectRequest.setRoot(rootDependency);
        for (RemoteRepository remoteRepository : remoteRepositories) {
            collectRequest.addRepository(remoteRepository);
        }

        DependencyRequest dependencyRequest = new DependencyRequest(
            collectRequest,
            DependencyFilterUtils.classpathFilter(JavaScopes.RUNTIME)
        );

        DependencyResult dependencyResult = repoSystem.resolveDependencies(session, dependencyRequest);
        List<ArtifactResult> artifactResults = dependencyResult.getArtifactResults();

        Set<Path> uniqueJars = new LinkedHashSet<>();
        Path resolvedRoot = null;
        for (ArtifactResult artifactResult : artifactResults) {
            if (artifactResult.getArtifact() == null || artifactResult.getArtifact().getFile() == null) {
                continue;
            }
            Path path = artifactResult.getArtifact().getFile().toPath().toAbsolutePath().normalize();
            uniqueJars.add(path);
            if (sameCoordinate(rootArtifact, artifactResult.getArtifact())) {
                resolvedRoot = path;
            }
        }

        if (resolvedRoot == null) {
            throw new InputException("Failed to resolve root recipe artifact: " + coords);
        }

        Path rootPath = resolvedRoot;
        List<Path> allJars = new ArrayList<>(uniqueJars);
        List<Path> deps = allJars.stream().filter(p -> !p.equals(rootPath)).collect(Collectors.toList());
        return new Resolution(rootPath, deps, allJars);
    }

    private static boolean sameCoordinate(org.eclipse.aether.artifact.Artifact left, org.eclipse.aether.artifact.Artifact right) {
        return Objects.equals(left.getGroupId(), right.getGroupId())
            && Objects.equals(left.getArtifactId(), right.getArtifactId())
            && Objects.equals(left.getVersion(), right.getVersion())
            && Objects.equals(left.getExtension(), right.getExtension())
            && Objects.equals(left.getClassifier(), right.getClassifier());
    }

    private static RepositorySystem newRepositorySystem() {
        RepositorySystem system = new RepositorySystemSupplier().get();
        if (system == null) {
            throw new IllegalStateException("RepositorySystem service not available");
        }
        return system;
    }

    private static boolean isTypeAttributionIssue(Throwable t) {
        Deque<Throwable> stack = new ArrayDeque<>();
        stack.push(t);
        while (!stack.isEmpty()) {
            Throwable next = stack.pop();
            String message = safeMessage(next).toLowerCase(Locale.ROOT);
            if (message.contains("type-attribution-unavailable")
                || message.contains("type attribution unavailable")
                || message.contains("missing or invalid type information")
                || message.contains("missing type information")) {
                return true;
            }
            if (next.getCause() != null && next.getCause() != next) {
                stack.push(next.getCause());
            }
            if (next.getSuppressed() != null) {
                for (Throwable suppressed : next.getSuppressed()) {
                    if (suppressed != null && suppressed != next) {
                        stack.push(suppressed);
                    }
                }
            }
        }
        return false;
    }

    private static String safeMessage(Throwable t) {
        return t.getMessage() == null ? t.getClass().getName() : t.getMessage();
    }

    private static final class Resolution {
        private final Path rootArtifact;
        private final List<Path> dependencyJars;
        private final List<Path> classpathJars;

        private Resolution(Path rootArtifact, List<Path> dependencyJars, List<Path> classpathJars) {
            this.rootArtifact = rootArtifact;
            this.dependencyJars = dependencyJars;
            this.classpathJars = classpathJars;
        }
    }

    private static final class IsolatedRewriteClassLoader extends URLClassLoader {
        private static final String REWRITE_PREFIX = "org.openrewrite.";
        private static final String RUNNER_PREFIX = "io.iw2rmb.ploy.orwcli.";

        private IsolatedRewriteClassLoader(URL[] urls, ClassLoader parent) {
            super(urls, parent);
        }

        @Override
        protected Class<?> loadClass(String name, boolean resolve) throws ClassNotFoundException {
            synchronized (getClassLoadingLock(name)) {
                Class<?> loaded = findLoadedClass(name);
                if (loaded != null) {
                    if (resolve) {
                        resolveClass(loaded);
                    }
                    return loaded;
                }

                if (isChildFirst(name)) {
                    try {
                        Class<?> childLoaded = findClass(name);
                        if (resolve) {
                            resolveClass(childLoaded);
                        }
                        return childLoaded;
                    } catch (ClassNotFoundException ignored) {
                        // fall back to parent loader below
                    }
                }
                return super.loadClass(name, resolve);
            }
        }

        private static boolean isChildFirst(String className) {
            return className.startsWith(REWRITE_PREFIX) || className.startsWith(RUNNER_PREFIX);
        }
    }

    private static final class CliOptions {
        private final Path dir;
        private final List<String> recipes;
        private final String coords;
        private final Path config;
        private final List<String> repos;
        private final String repoUsername;
        private final String repoPassword;
        private final String buildSystem;
        private final List<PathMatcher> excludePathMatchers;

        private CliOptions(
            Path dir,
            List<String> recipes,
            String coords,
            Path config,
            List<String> repos,
            String repoUsername,
            String repoPassword,
            String buildSystem,
            List<PathMatcher> excludePathMatchers
        ) {
            this.dir = dir;
            this.recipes = recipes;
            this.coords = coords;
            this.config = config;
            this.repos = repos;
            this.repoUsername = repoUsername;
            this.repoPassword = repoPassword;
            this.buildSystem = buildSystem;
            this.excludePathMatchers = excludePathMatchers;
        }

        private static CliOptions parse(String[] args) {
            Path dir = null;
            List<String> recipes = new ArrayList<>();
            String coords = null;
            Path config = null;
            List<String> repos = new ArrayList<>();
            String repoUsername = null;
            String repoPassword = null;
            String buildSystem = null;
            List<PathMatcher> excludePathMatchers = Collections.emptyList();
            boolean apply = false;

            for (int i = 0; i < args.length; i++) {
                String arg = args[i];
                switch (arg) {
                    case "--apply":
                        apply = true;
                        break;
                    case "--dir":
                        dir = Paths.get(requireValue(args, ++i, "--dir"));
                        break;
                    case "--recipe":
                        recipes.addAll(splitCsv(requireValue(args, ++i, "--recipe")));
                        break;
                    case "--coords":
                        coords = requireValue(args, ++i, "--coords");
                        break;
                    case "--config":
                        config = Paths.get(requireValue(args, ++i, "--config"));
                        break;
                    case "--repo":
                        repos.add(requireValue(args, ++i, "--repo"));
                        break;
                    case "--repo-username":
                        repoUsername = requireValue(args, ++i, "--repo-username");
                        break;
                    case "--repo-password":
                        repoPassword = requireValue(args, ++i, "--repo-password");
                        break;
                    case "--build-system":
                        buildSystem = requireValue(args, ++i, "--build-system");
                        break;
                    default:
                        throw new InputException("unknown arg: " + arg);
                }
            }

            if (!apply) {
                throw new InputException("--apply is required");
            }
            if (dir == null) {
                throw new InputException("--dir is required");
            }
            if (coords == null || coords.isBlank()) {
                throw new InputException("--coords is required");
            }
            if (recipes.isEmpty()) {
                throw new InputException("--recipe is required");
            }
            if ((repoUsername == null) != (repoPassword == null)) {
                throw new InputException("--repo-username and --repo-password must be provided together");
            }
            excludePathMatchers = compileExcludePathMatchers(System.getenv(EXCLUDE_PATHS_ENV));

            return new CliOptions(
                dir,
                Collections.unmodifiableList(recipes),
                coords,
                config,
                Collections.unmodifiableList(repos),
                repoUsername,
                repoPassword,
                buildSystem,
                excludePathMatchers
            );
        }

        private static String requireValue(String[] args, int index, String flag) {
            if (index >= args.length) {
                throw new InputException("missing value for " + flag);
            }
            return args[index];
        }

        private static List<String> splitCsv(String raw) {
            if (raw == null) {
                return Collections.emptyList();
            }
            return Arrays.stream(raw.split(","))
                .map(String::trim)
                .filter(s -> !s.isEmpty())
                .collect(Collectors.toList());
        }
    }

    private static final class InputException extends RuntimeException {
        private InputException(String message) {
            super(message);
        }
    }

    private static final class UnsupportedTypeAttributionException extends RuntimeException {
        private UnsupportedTypeAttributionException(String message, Throwable cause) {
            super(message, cause);
        }
    }
}
