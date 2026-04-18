import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Arrays;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.jar.JarEntry;
import java.util.jar.JarOutputStream;
import org.junit.jupiter.api.Test;

class JapicmpRemovalJavadocEnricherCliTest {
    private static final ObjectMapper JSON = new ObjectMapper();

    @Test
    void enrichRemovalsAddsLatestDeprecatedNoteAcrossRemovalKinds()
        throws Exception {
        Path tempDir = Files.createTempDirectory("japi-javadoc-enricher-test");

        Path v100 = createSourcesJar(
            tempDir.resolve("acme-1.0.0-sources.jar"),
            sources("v1.0.0")
        );
        Path v110 = createSourcesJar(
            tempDir.resolve("acme-1.1.0-sources.jar"),
            sources("v1.1.0")
        );

        Map<String, Path> sourcesByVersion = new HashMap<String, Path>();
        sourcesByVersion.put("1.0.0", v100);
        sourcesByVersion.put("1.1.0", v110);

        ArrayNode removals = JSON.createArrayNode();
        removals.add(
            removal(
                "method",
                "com.acme.Api",
                "oldMethod(java.lang.String)",
                "METHOD_REMOVED"
            )
        );
        removals.add(
            removal(
                "constructor",
                "com.acme.Api",
                "Api(java.lang.String)",
                "CONSTRUCTOR_REMOVED"
            )
        );
        removals.add(
            removal("field", "com.acme.Api", "oldField", "FIELD_REMOVED")
        );
        removals.add(removal("class", "com.acme.Api", null, "CLASS_REMOVED"));
        removals.add(
            removal(
                "interface",
                "com.acme.Child",
                "com.acme.OldInterface",
                "INTERFACE_REMOVED"
            )
        );
        removals.add(
            removal(
                "superclass",
                "com.acme.Child",
                "com.acme.OldBase",
                "SUPERCLASS_REMOVED"
            )
        );
        removals.add(
            removal(
                "method",
                "com.acme.Api",
                "missing(java.lang.String)",
                "METHOD_REMOVED"
            )
        );

        ArrayNode enriched = JapicmpRemovalJavadocEnricherCli.enrichRemovals(
            removals,
            Arrays.asList("1.2.0", "1.1.0", "1.0.0"),
            version -> sourcesByVersion.get(version)
        );

        Map<Integer, ExpectedRow> expected = new LinkedHashMap<Integer, ExpectedRow>();
        expected.put(0, new ExpectedRow("1.1.0", "method note v1.1.0"));
        expected.put(1, new ExpectedRow("1.1.0", "constructor note v1.1.0"));
        expected.put(2, new ExpectedRow("1.1.0", "field note v1.1.0"));
        expected.put(3, new ExpectedRow("1.1.0", "type note v1.1.0"));
        expected.put(4, new ExpectedRow("1.1.0", "interface note v1.1.0"));
        expected.put(5, new ExpectedRow("1.1.0", "superclass note v1.1.0"));
        expected.put(6, new ExpectedRow(null, null));

        for (Map.Entry<Integer, ExpectedRow> row : expected.entrySet()) {
            ObjectNode actual = (ObjectNode) enriched.get(row.getKey().intValue());
            assertTrue(actual.has("javadoc_last_ver"));
            assertTrue(actual.has("javadoc_last_note"));
            assertEquals(
                row.getValue().version,
                textOrNull(actual, "javadoc_last_ver")
            );
            assertEquals(row.getValue().note, textOrNull(actual, "javadoc_last_note"));
        }
    }

    private static ObjectNode removal(
        String kind,
        String className,
        String member,
        String type
    ) {
        ObjectNode node = JSON.createObjectNode();
        node.put("kind", kind);
        node.put("class", className);
        if (member == null) {
            node.putNull("member");
        } else {
            node.put("member", member);
        }
        node.put("type", type);
        node.put("binary_compatible", false);
        node.put("source_compatible", false);
        return node;
    }

    private static String textOrNull(ObjectNode node, String field) {
        return node.get(field).isNull() ? null : node.get(field).asText();
    }

    private static Map<String, String> sources(String label) {
        Map<String, String> files = new LinkedHashMap<String, String>();
        files.put(
            "com/acme/Api.java",
            "package com.acme;\n" +
            "/** @deprecated type note " + label + " */\n" +
            "@Deprecated\n" +
            "public class Api {\n" +
            "  /** @deprecated field note " + label + " */\n" +
            "  @Deprecated\n" +
            "  public String oldField;\n" +
            "  /** @deprecated constructor note " + label + " */\n" +
            "  @Deprecated\n" +
            "  public Api(String value) {}\n" +
            "  /** @deprecated method note " + label + " */\n" +
            "  @Deprecated\n" +
            "  public void oldMethod(String value) {}\n" +
            "}\n"
        );
        files.put(
            "com/acme/OldInterface.java",
            "package com.acme;\n" +
            "/** @deprecated interface note " + label + " */\n" +
            "@Deprecated\n" +
            "public interface OldInterface {}\n"
        );
        files.put(
            "com/acme/OldBase.java",
            "package com.acme;\n" +
            "/** @deprecated superclass note " + label + " */\n" +
            "@Deprecated\n" +
            "public class OldBase {}\n"
        );
        return files;
    }

    private static Path createSourcesJar(Path jarPath, Map<String, String> files)
        throws IOException {
        Files.createDirectories(jarPath.getParent());
        try (JarOutputStream out = new JarOutputStream(Files.newOutputStream(jarPath))) {
            for (Map.Entry<String, String> file : files.entrySet()) {
                out.putNextEntry(new JarEntry(file.getKey()));
                out.write(file.getValue().getBytes(StandardCharsets.UTF_8));
                out.closeEntry();
            }
        }
        return jarPath;
    }

    private static final class ExpectedRow {
        private final String version;
        private final String note;

        private ExpectedRow(String version, String note) {
            this.version = version;
            this.note = note;
        }
    }
}
