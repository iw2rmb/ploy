import com.github.javaparser.JavaParser;
import com.github.javaparser.ParseResult;
import com.github.javaparser.ParserConfiguration;
import com.github.javaparser.ast.CompilationUnit;
import com.github.javaparser.ast.ImportDeclaration;
import com.github.javaparser.ast.NodeList;
import com.github.javaparser.ast.body.BodyDeclaration;
import com.github.javaparser.ast.body.ConstructorDeclaration;
import com.github.javaparser.ast.body.FieldDeclaration;
import com.github.javaparser.ast.body.MethodDeclaration;
import com.github.javaparser.ast.body.TypeDeclaration;
import com.github.javaparser.ast.comments.JavadocComment;
import com.github.javaparser.ast.expr.AnnotationExpr;
import com.github.javaparser.ast.expr.MemberValuePair;
import com.github.javaparser.ast.expr.NormalAnnotationExpr;
import com.github.javaparser.ast.expr.StringLiteralExpr;
import com.github.javaparser.ast.nodeTypes.NodeWithAnnotations;
import com.github.javaparser.ast.nodeTypes.NodeWithJavadoc;
import com.github.javaparser.ast.type.PrimitiveType;
import com.github.javaparser.ast.type.Type;
import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.Enumeration;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Optional;
import java.util.Set;
import java.util.jar.JarEntry;
import java.util.jar.JarFile;

final class SourceDeprecationScanner {
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

    SourceDeprecationCatalog scanSourcesJar(Path sourcesJar) throws IOException {
        if (sourcesJar == null) {
            return SourceDeprecationCatalog.empty();
        }
        if (!java.nio.file.Files.isRegularFile(sourcesJar)) {
            return SourceDeprecationCatalog.empty();
        }

        SourceDeprecationCatalogBuilder builder =
            new SourceDeprecationCatalogBuilder();
        ParserConfiguration parserConfiguration = new ParserConfiguration();
        parserConfiguration.setLanguageLevel(
            ParserConfiguration.LanguageLevel.BLEEDING_EDGE
        );
        JavaParser parser = new JavaParser(parserConfiguration);

        try (JarFile jarFile = new JarFile(sourcesJar.toFile())) {
            Enumeration<JarEntry> entries = jarFile.entries();
            while (entries.hasMoreElements()) {
                JarEntry entry = entries.nextElement();
                if (entry.isDirectory() || !entry.getName().endsWith(".java")) {
                    continue;
                }
                CompilationUnit unit = parseCompilationUnit(parser, jarFile, entry);
                if (unit == null) {
                    continue;
                }
                indexCompilationUnit(unit, builder);
            }
        }

        return builder.build();
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

    private static void indexCompilationUnit(
        CompilationUnit compilationUnit,
        SourceDeprecationCatalogBuilder builder
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
            indexTypeDeclaration(
                type,
                packageName,
                null,
                typeNameContext,
                builder
            );
        }
    }

    private static void indexTypeDeclaration(
        TypeDeclaration<?> type,
        String packageName,
        String outerClassName,
        TypeNameContext typeNameContext,
        SourceDeprecationCatalogBuilder builder
    ) {
        String className =
            outerClassName == null
                ? qualify(packageName, type.getNameAsString())
                : outerClassName + "." + type.getNameAsString();
        className = normalizeClassName(className);

        DeprecationPayload classPayload = deprecationPayload(type);
        if (classPayload.isDeprecated()) {
            builder.put(
                className,
                classPayload.note,
                classPayload.since
            );
        }

        if (type.getMembers().isEmpty()) {
            return;
        }

        for (BodyDeclaration<?> member : type.getMembers()) {
            if (member instanceof MethodDeclaration) {
                MethodDeclaration method = (MethodDeclaration) member;
                DeprecationPayload payload = deprecationPayload(method);
                if (!payload.isDeprecated()) {
                    continue;
                }
                List<String> parameterTypes = parameterTypes(
                    method.getParameters(),
                    typeNameContext
                );
                builder.put(
                    className +
                    "#" +
                    method.getNameAsString() +
                    "(" +
                    String.join(",", parameterTypes) +
                    ")",
                    payload.note,
                    payload.since
                );
                continue;
            }

            if (member instanceof ConstructorDeclaration) {
                ConstructorDeclaration constructor = (ConstructorDeclaration) member;
                DeprecationPayload payload = deprecationPayload(constructor);
                if (!payload.isDeprecated()) {
                    continue;
                }
                List<String> parameterTypes = parameterTypes(
                    constructor.getParameters(),
                    typeNameContext
                );
                builder.put(
                    className +
                    "#<init>(" +
                    String.join(",", parameterTypes) +
                    ")",
                    payload.note,
                    payload.since
                );
                continue;
            }

            if (member instanceof FieldDeclaration) {
                FieldDeclaration field = (FieldDeclaration) member;
                DeprecationPayload payload = deprecationPayload(field);
                if (!payload.isDeprecated()) {
                    continue;
                }
                for (int i = 0; i < field.getVariables().size(); i++) {
                    builder.put(
                        className + "#" + field.getVariables().get(i).getNameAsString(),
                        payload.note,
                        payload.since
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
                    builder
                );
            }
        }
    }

    private static DeprecationPayload deprecationPayload(BodyDeclaration<?> node) {
        String note = deprecatedNote(node);
        String sinceFromJavadoc = sinceTag(node);

        String annotationSince = null;
        boolean hasDeprecatedAnnotation = false;
        if (node instanceof NodeWithAnnotations<?>) {
            NodeWithAnnotations<?> withAnnotations = (NodeWithAnnotations<?>) node;
            for (AnnotationExpr annotation : withAnnotations.getAnnotations()) {
                String name = annotation.getNameAsString();
                if (
                    "Deprecated".equals(name) ||
                    "java.lang.Deprecated".equals(name)
                ) {
                    hasDeprecatedAnnotation = true;
                    annotationSince = deprecatedAnnotationSince(annotation);
                    break;
                }
            }
        }

        return new DeprecationPayload(
            hasDeprecatedAnnotation || note != null,
            note,
            firstNonBlank(annotationSince, sinceFromJavadoc)
        );
    }

    private static String deprecatedAnnotationSince(AnnotationExpr annotation) {
        if (!(annotation instanceof NormalAnnotationExpr)) {
            return null;
        }
        NormalAnnotationExpr normal = (NormalAnnotationExpr) annotation;
        for (MemberValuePair pair : normal.getPairs()) {
            if (!"since".equals(pair.getNameAsString())) {
                continue;
            }
            if (pair.getValue() instanceof StringLiteralExpr) {
                String value = ((StringLiteralExpr) pair.getValue()).asString();
                return normalizeWhitespace(value);
            }
            return normalizeWhitespace(pair.getValue().toString());
        }
        return null;
    }

    private static String deprecatedNote(BodyDeclaration<?> node) {
        if (!(node instanceof NodeWithJavadoc<?>)) {
            return null;
        }
        NodeWithJavadoc<?> withJavadoc = (NodeWithJavadoc<?>) node;
        Optional<JavadocComment> comment = withJavadoc.getJavadocComment();
        if (!comment.isPresent()) {
            return null;
        }
        return extractTag(comment.get().getContent(), "deprecated");
    }

    private static String sinceTag(BodyDeclaration<?> node) {
        if (!(node instanceof NodeWithJavadoc<?>)) {
            return null;
        }
        NodeWithJavadoc<?> withJavadoc = (NodeWithJavadoc<?>) node;
        Optional<JavadocComment> comment = withJavadoc.getJavadocComment();
        if (!comment.isPresent()) {
            return null;
        }
        return extractTag(comment.get().getContent(), "since");
    }

    private static String extractTag(String content, String tagName) {
        if (content == null || content.trim().isEmpty()) {
            return null;
        }
        String tag = "@" + tagName;
        String[] lines = content.replace("\r", "").split("\n");
        StringBuilder captured = new StringBuilder();
        boolean collecting = false;
        for (String rawLine : lines) {
            String line = rawLine.trim();
            if (line.startsWith("*")) {
                line = line.substring(1).trim();
            }
            if (!collecting) {
                int idx = line.indexOf(tag);
                if (idx < 0) {
                    continue;
                }
                collecting = true;
                String tail = line.substring(idx + tag.length()).trim();
                if (!tail.isEmpty()) {
                    appendSpaceSeparated(captured, tail);
                }
                continue;
            }
            if (line.startsWith("@")) {
                break;
            }
            if (line.isEmpty()) {
                continue;
            }
            appendSpaceSeparated(captured, line);
        }

        String normalized = normalizeWhitespace(captured.toString());
        return normalized.isEmpty() ? null : normalized;
    }

    private static void appendSpaceSeparated(StringBuilder out, String value) {
        if (value == null || value.trim().isEmpty()) {
            return;
        }
        if (out.length() > 0) {
            out.append(' ');
        }
        out.append(value.trim());
    }

    private static String firstNonBlank(String first, String second) {
        if (first != null && !first.trim().isEmpty()) {
            return first.trim();
        }
        if (second != null && !second.trim().isEmpty()) {
            return second.trim();
        }
        return null;
    }

    private static String normalizeWhitespace(String value) {
        if (value == null) {
            return "";
        }
        return value.trim().replaceAll("\\s+", " ");
    }

    private static String qualify(String packageName, String simpleName) {
        if (packageName == null || packageName.isEmpty()) {
            return simpleName;
        }
        return packageName + "." + simpleName;
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
        if (!base.contains(".") && isTypeVariableName(base)) {
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

        if (!base.contains(".") && JAVA_LANG_TYPES.contains(firstSegment)) {
            return "java.lang." + base;
        }

        if (!base.contains(".") && !typeNameContext.wildcardImports.isEmpty()) {
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

    private static String normalizeClassName(String className) {
        if (className == null) {
            return "";
        }
        return className.replace('$', '.').trim();
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

    private static boolean isTypeVariableName(String value) {
        if (value == null || value.isEmpty()) {
            return false;
        }
        String trimmed = value.trim();
        if (trimmed.isEmpty()) {
            return false;
        }
        if (!isTypeVariableTokenChars(trimmed)) {
            return false;
        }
        if (!Character.isUpperCase(trimmed.charAt(0))) {
            return false;
        }
        if (trimmed.length() == 1) {
            return true;
        }
        if (trimmed.endsWith("T") && trimmed.length() <= 8) {
            return true;
        }
        return false;
    }

    private static boolean isTypeVariableTokenChars(String trimmed) {
        for (int i = 0; i < trimmed.length(); i++) {
            char ch = trimmed.charAt(i);
            if (Character.isLetterOrDigit(ch) || ch == '_' || ch == '$') {
                continue;
            }
            return false;
        }
        return true;
    }

    static final class SourceDeprecationCatalog {
        private static final SourceDeprecationCatalog EMPTY =
            new SourceDeprecationCatalog(Collections.emptyMap());

        private final Map<String, DeprecationEntry> deprecatedBySymbol;

        private SourceDeprecationCatalog(Map<String, DeprecationEntry> deprecatedBySymbol) {
            this.deprecatedBySymbol = deprecatedBySymbol;
        }

        static SourceDeprecationCatalog empty() {
            return EMPTY;
        }

        Map<String, DeprecationEntry> getDeprecatedBySymbol() {
            return deprecatedBySymbol;
        }

        boolean hasSymbol(String symbol) {
            return deprecatedBySymbol.containsKey(symbol);
        }

        DeprecationEntry get(String symbol) {
            return deprecatedBySymbol.get(symbol);
        }

        Set<String> symbols() {
            return deprecatedBySymbol.keySet();
        }
    }

    static final class DeprecationEntry {
        private final String note;
        private final String since;

        private DeprecationEntry(String note, String since) {
            this.note = firstNonBlank(note, null);
            this.since = firstNonBlank(since, null);
        }

        String getNote() {
            return note;
        }

        String getSince() {
            return since;
        }
    }

    private static final class SourceDeprecationCatalogBuilder {
        private final Map<String, DeprecationEntry> deprecatedBySymbol =
            new LinkedHashMap<String, DeprecationEntry>();

        void put(String symbol, String note, String since) {
            if (symbol == null || symbol.trim().isEmpty()) {
                return;
            }
            String key = symbol.trim();
            DeprecationEntry existing = deprecatedBySymbol.get(key);
            if (existing == null) {
                deprecatedBySymbol.put(key, new DeprecationEntry(note, since));
                return;
            }
            String mergedNote = firstNonBlank(existing.getNote(), note);
            String mergedSince = firstNonBlank(existing.getSince(), since);
            deprecatedBySymbol.put(key, new DeprecationEntry(mergedNote, mergedSince));
        }

        SourceDeprecationCatalog build() {
            if (deprecatedBySymbol.isEmpty()) {
                return SourceDeprecationCatalog.empty();
            }
            return new SourceDeprecationCatalog(
                Collections.unmodifiableMap(
                    new LinkedHashMap<String, DeprecationEntry>(deprecatedBySymbol)
                )
            );
        }
    }

    private static final class DeprecationPayload {
        private final boolean deprecated;
        private final String note;
        private final String since;

        private DeprecationPayload(boolean deprecated, String note, String since) {
            this.deprecated = deprecated;
            this.note = note;
            this.since = since;
        }

        boolean isDeprecated() {
            return deprecated;
        }
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
}
