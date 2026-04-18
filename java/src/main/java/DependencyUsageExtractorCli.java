import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;

public final class DependencyUsageExtractorCli {
    private DependencyUsageExtractorCli() {}

    public static void main(String[] args) throws Exception {
        try {
            CliOptions options = CliOptions.parse(args);
            if (options.showHelp) {
                printUsage();
                return;
            }

            DependencyUsageExtractor extractor = new DependencyUsageExtractor();
            DependencyUsageExtractor.Result result = extractor.analyze(
                new DependencyUsageExtractor.Config(
                    options.repoPath,
                    options.classpathFile,
                    options.targetPackages
                )
            );

            String json = DependencyUsageJsonWriter.toJson(result);
            if (options.outputFile == null) {
                System.out.print(json);
                return;
            }

            Path parent = options.outputFile.getParent();
            if (parent != null) {
                Files.createDirectories(parent);
            }
            Files.write(options.outputFile, json.getBytes(StandardCharsets.UTF_8));
        } catch (IllegalArgumentException ex) {
            System.err.println(ex.getMessage());
            printUsage();
            System.exit(1);
        } catch (Exception ex) {
            System.err.println(ex.getMessage());
            System.exit(1);
        }
    }

    private static void printUsage() {
        System.err.println(
            "Usage: DependencyUsageExtractorCli --repo <path> " +
            "--classpath-file <path> [--target-package <prefix> " +
            "--target-package <prefix> ... | --no-target-filter] " +
            "[--output <path>]"
        );
    }

    private static final class CliOptions {
        private final boolean showHelp;
        private final Path repoPath;
        private final Path classpathFile;
        private final List<String> targetPackages;
        private final boolean noTargetFilter;
        private final Path outputFile;

        private CliOptions(
            boolean showHelp,
            Path repoPath,
            Path classpathFile,
            List<String> targetPackages,
            boolean noTargetFilter,
            Path outputFile
        ) {
            this.showHelp = showHelp;
            this.repoPath = repoPath;
            this.classpathFile = classpathFile;
            this.targetPackages = targetPackages;
            this.noTargetFilter = noTargetFilter;
            this.outputFile = outputFile;
        }

        private static CliOptions parse(String[] args) {
            if (args == null || args.length == 0) {
                throw new IllegalArgumentException("Missing arguments");
            }

            Path repoPath = null;
            Path classpathFile = null;
            Path outputFile = null;
            boolean noTargetFilter = false;
            List<String> targetPackages = new ArrayList<String>();

            for (int i = 0; i < args.length; i++) {
                String arg = args[i];
                if ("--help".equals(arg) || "-h".equals(arg)) {
                    return new CliOptions(
                        true,
                        null,
                        null,
                        new ArrayList<String>(),
                        false,
                        null
                    );
                }
                if ("--repo".equals(arg)) {
                    repoPath = valueOfPath(args, ++i, arg);
                    continue;
                }
                if ("--classpath-file".equals(arg)) {
                    classpathFile = valueOfPath(args, ++i, arg);
                    continue;
                }
                if ("--target-package".equals(arg)) {
                    targetPackages.add(valueOfString(args, ++i, arg));
                    continue;
                }
                if ("--output".equals(arg)) {
                    outputFile = valueOfPath(args, ++i, arg);
                    continue;
                }
                if ("--no-target-filter".equals(arg)) {
                    noTargetFilter = true;
                    continue;
                }

                throw new IllegalArgumentException("Unknown argument: " + arg);
            }

            if (repoPath == null) {
                throw new IllegalArgumentException("Missing required --repo");
            }
            if (classpathFile == null) {
                throw new IllegalArgumentException(
                    "Missing required --classpath-file"
                );
            }
            if (noTargetFilter && !targetPackages.isEmpty()) {
                throw new IllegalArgumentException(
                    "Use either --target-package or --no-target-filter, not both"
                );
            }
            if (!noTargetFilter && targetPackages.isEmpty()) {
                throw new IllegalArgumentException(
                    "At least one --target-package is required unless --no-target-filter is set"
                );
            }

            return new CliOptions(
                false,
                repoPath,
                classpathFile,
                targetPackages,
                noTargetFilter,
                outputFile
            );
        }

        private static Path valueOfPath(String[] args, int index, String flag) {
            return Path.of(valueOfString(args, index, flag));
        }

        private static String valueOfString(
            String[] args,
            int index,
            String flag
        ) {
            if (index >= args.length) {
                throw new IllegalArgumentException("Missing value for " + flag);
            }
            return args[index];
        }
    }
}
