import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;

public final class DependencyDeprecatedUsageReportCli {
    private DependencyDeprecatedUsageReportCli() {}

    public static void main(String[] args) throws Exception {
        try {
            CliOptions options = CliOptions.parse(args);
            if (options.showHelp) {
                printUsage();
                return;
            }

            DependencyDeprecatedUsageReportComposer composer =
                new DependencyDeprecatedUsageReportComposer();
            List<DependencyDeprecatedUsageReportComposer.ReportGroup> report =
                composer.compose(
                    new DependencyDeprecatedUsageReportComposer.Config(
                        options.usageReportFile,
                        options.classpathFile,
                        options.repoUrl,
                        options.outputFile != null
                    )
                );

            String json = DependencyDeprecatedUsageReportComposer.toJson(report);
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
            "Usage: DependencyDeprecatedUsageReportCli " +
            "--usage-report <path> --classpath-file <path> [--output <path>] [--repo-url <url>]"
        );
    }

    private static final class CliOptions {
        private final boolean showHelp;
        private final Path usageReportFile;
        private final Path classpathFile;
        private final Path outputFile;
        private final String repoUrl;

        private CliOptions(
            boolean showHelp,
            Path usageReportFile,
            Path classpathFile,
            Path outputFile,
            String repoUrl
        ) {
            this.showHelp = showHelp;
            this.usageReportFile = usageReportFile;
            this.classpathFile = classpathFile;
            this.outputFile = outputFile;
            this.repoUrl = repoUrl;
        }

        private static CliOptions parse(String[] args) {
            if (args == null || args.length == 0) {
                throw new IllegalArgumentException("Missing arguments");
            }

            Path usageReportFile = null;
            Path classpathFile = null;
            Path outputFile = null;
            String repoUrl = null;

            for (int i = 0; i < args.length; i++) {
                String arg = args[i];
                if ("--help".equals(arg) || "-h".equals(arg)) {
                    return new CliOptions(true, null, null, null, null);
                }
                if ("--usage-report".equals(arg)) {
                    usageReportFile = valueOfPath(args, ++i, arg);
                    continue;
                }
                if ("--classpath-file".equals(arg)) {
                    classpathFile = valueOfPath(args, ++i, arg);
                    continue;
                }
                if ("--output".equals(arg)) {
                    outputFile = valueOfPath(args, ++i, arg);
                    continue;
                }
                if ("--repo-url".equals(arg)) {
                    repoUrl = valueOfString(args, ++i, arg);
                    continue;
                }

                throw new IllegalArgumentException("Unknown argument: " + arg);
            }

            if (usageReportFile == null) {
                throw new IllegalArgumentException("Missing required --usage-report");
            }
            if (classpathFile == null) {
                throw new IllegalArgumentException("Missing required --classpath-file");
            }

            return new CliOptions(
                false,
                usageReportFile,
                classpathFile,
                outputFile,
                repoUrl
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
