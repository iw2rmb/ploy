import java.util.List;

public final class DependencyUsageJsonWriter {
    private DependencyUsageJsonWriter() {}

    public static String toJson(DependencyUsageExtractor.Result result) {
        StringBuilder out = new StringBuilder();
        out.append("{\n");
        out.append("  \"usages\": [");

        List<DependencyUsageExtractor.UsageGroup> usages = result.getUsages();
        if (!usages.isEmpty()) {
            out.append("\n");
            for (int i = 0; i < usages.size(); i++) {
                DependencyUsageExtractor.UsageGroup usage = usages.get(i);
                out.append("    {\n");
                out
                    .append("      \"package\": \"")
                    .append(escape(usage.getDependencyPackage()))
                    .append("\",\n");
                out
                    .append("      \"groupId\": \"")
                    .append(escape(usage.getGroupId()))
                    .append("\",\n");
                out
                    .append("      \"artifactId\": \"")
                    .append(escape(usage.getArtifactId()))
                    .append("\",\n");
                out
                    .append("      \"version\": \"")
                    .append(escape(usage.getVersion()))
                    .append("\",\n");
                out.append("      \"symbols\": [");

                List<String> symbols = usage.getSymbols();
                if (!symbols.isEmpty()) {
                    out.append("\n");
                    for (int s = 0; s < symbols.size(); s++) {
                        out
                            .append("        \"")
                            .append(escape(symbols.get(s)))
                            .append("\"");
                        if (s + 1 < symbols.size()) {
                            out.append(",");
                        }
                        out.append("\n");
                    }
                    out.append("      ]\n");
                } else {
                    out.append("]\n");
                }

                out.append("    }");
                if (i + 1 < usages.size()) {
                    out.append(",");
                }
                out.append("\n");
            }
            out.append("  ]\n");
        } else {
            out.append("]\n");
        }

        out.append("}\n");
        return out.toString();
    }

    private static String escape(String input) {
        if (input == null) {
            return "";
        }
        StringBuilder escaped = new StringBuilder();
        for (int i = 0; i < input.length(); i++) {
            char ch = input.charAt(i);
            switch (ch) {
                case '"':
                    escaped.append("\\\"");
                    break;
                case '\\':
                    escaped.append("\\\\");
                    break;
                case '\b':
                    escaped.append("\\b");
                    break;
                case '\f':
                    escaped.append("\\f");
                    break;
                case '\n':
                    escaped.append("\\n");
                    break;
                case '\r':
                    escaped.append("\\r");
                    break;
                case '\t':
                    escaped.append("\\t");
                    break;
                default:
                    if (ch < 0x20) {
                        escaped.append(String.format("\\u%04x", (int) ch));
                    } else {
                        escaped.append(ch);
                    }
                    break;
            }
        }
        return escaped.toString();
    }
}
