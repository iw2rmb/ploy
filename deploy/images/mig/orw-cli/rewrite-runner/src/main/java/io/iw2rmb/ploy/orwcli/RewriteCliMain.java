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
import org.openrewrite.maven.MavenParser;
import org.openrewrite.polyglot.OmniParser;

import java.io.IOException;
import java.io.InputStream;
import java.net.URL;
import java.net.URLClassLoader;
import java.nio.file.Files;
import java.nio.file.Path;
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

public final class RewriteCliMain {
    private static final String DEFAULT_REPO = "https://repo1.maven.org/maven2/";

    private RewriteCliMain() {
    }

    public static void main(String[] args) {
        try {
            CliOptions opts = CliOptions.parse(args);
            run(opts);
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

        List<SourceFile> sourceFiles = parseWorkspace(workspace, ctx);
        InMemoryLargeSourceSet before = new InMemoryLargeSourceSet(sourceFiles, recipeClassLoader);

        RecipeRun run = recipe.run(before, ctx);
        List<Result> results = run.getChangeset().getAllResults();

        applyResults(workspace, results);

        System.out.println("[rewrite] Parsed files: " + sourceFiles.size());
        System.out.println("[rewrite] Applied results: " + results.size());
    }

    private static Recipe loadRecipe(CliOptions opts, Resolution resolution, ClassLoader recipeClassLoader) throws IOException {
        Properties properties = new Properties();
        Environment.Builder builder = Environment.builder(properties).scanRuntimeClasspath();

        if (opts.config != null) {
            Path configPath = opts.config.toAbsolutePath().normalize();
            if (!Files.isRegularFile(configPath)) {
                throw new InputException("--config file does not exist: " + configPath);
            }
            try (InputStream is = Files.newInputStream(configPath)) {
                builder.load(new YamlResourceLoader(is, configPath.toUri(), properties));
            }
        }

        if (resolution.rootArtifact != null) {
            builder.scanJar(resolution.rootArtifact, resolution.dependencyJars, recipeClassLoader);
        }

        Recipe recipe = builder.build().activateRecipes(opts.recipes);
        if (!recipe.validate().isValid()) {
            throw new InputException("Recipe validation failed for active recipes: " + String.join(",", opts.recipes));
        }
        return recipe;
    }

    private static List<SourceFile> parseWorkspace(Path workspace, ExecutionContext ctx) {
        List<Parser> parsers = new ArrayList<>();
        parsers.add(MavenParser.builder().build());
        parsers.add(JavaParser.fromJavaVersion().logCompilationWarningsAndErrors(true).build());
        parsers.addAll(OmniParser.defaultResourceParsers());

        OmniParser parser = OmniParser.builder(parsers).build();
        List<Path> accepted = parser.acceptedPaths(workspace);
        return parser.parse(accepted, workspace, ctx).collect(Collectors.toList());
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

    private static final class CliOptions {
        private final Path dir;
        private final List<String> recipes;
        private final String coords;
        private final Path config;
        private final List<String> repos;
        private final String repoUsername;
        private final String repoPassword;

        private CliOptions(
            Path dir,
            List<String> recipes,
            String coords,
            Path config,
            List<String> repos,
            String repoUsername,
            String repoPassword
        ) {
            this.dir = dir;
            this.recipes = recipes;
            this.coords = coords;
            this.config = config;
            this.repos = repos;
            this.repoUsername = repoUsername;
            this.repoPassword = repoPassword;
        }

        private static CliOptions parse(String[] args) {
            Path dir = null;
            List<String> recipes = new ArrayList<>();
            String coords = null;
            Path config = null;
            List<String> repos = new ArrayList<>();
            String repoUsername = null;
            String repoPassword = null;
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

            return new CliOptions(
                dir,
                Collections.unmodifiableList(recipes),
                coords,
                config,
                Collections.unmodifiableList(repos),
                repoUsername,
                repoPassword
            );
        }

        private static String requireValue(String[] args, int index, String flag) {
            if (index >= args.length) {
                throw new InputException("missing value for " + flag);
            }
            return args[index];
        }

        private static List<String> splitCsv(String raw) {
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
