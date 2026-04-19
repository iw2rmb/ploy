import java.io.IOException;
import java.io.InputStream;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.jar.JarEntry;
import java.util.jar.JarFile;
import org.objectweb.asm.AnnotationVisitor;
import org.objectweb.asm.ClassReader;
import org.objectweb.asm.ClassVisitor;
import org.objectweb.asm.FieldVisitor;
import org.objectweb.asm.MethodVisitor;
import org.objectweb.asm.Opcodes;
import org.objectweb.asm.Type;

final class BytecodeDeprecationScanner {
    private static final String DEPRECATED_DESCRIPTOR = "Ljava/lang/Deprecated;";

    private final Map<String, ClassLocation> classLocations =
        new LinkedHashMap<String, ClassLocation>();
    private final Map<String, ClassDeprecationInfo> classInfoByName =
        new HashMap<String, ClassDeprecationInfo>();

    BytecodeDeprecationScanner(List<Path> jarPaths) throws IOException {
        indexClassLocations(jarPaths);
    }

    boolean isDeprecated(DependencyDeprecatedUsageReportComposer.SymbolRef symbol)
        throws IOException {
        if (symbol == null) {
            return false;
        }
        return isDeprecatedCanonical(symbol.toCanonicalSymbol());
    }

    boolean isDeprecatedCanonical(String canonicalSymbol) throws IOException {
        if (canonicalSymbol == null || canonicalSymbol.trim().isEmpty()) {
            return false;
        }
        DependencyDeprecatedUsageReportComposer.SymbolRef symbol =
            DependencyDeprecatedUsageReportComposer.SymbolRef.parse(canonicalSymbol);
        if (symbol == null) {
            return false;
        }

        ClassDeprecationInfo classInfo = loadClassInfo(symbol.getOwnerClass());
        if (classInfo == null) {
            return false;
        }

        switch (symbol.getKind()) {
            case TYPE:
                return classInfo.classDeprecated;
            case FIELD:
                return classInfo.deprecatedFields.containsKey(symbol.getMemberName());
            case METHOD:
                return isDeprecatedMethod(
                    classInfo.deprecatedMethods,
                    symbol.getMemberName(),
                    symbol.getParameterTypes()
                );
            case CONSTRUCTOR:
                return isDeprecatedMethod(
                    classInfo.deprecatedMethods,
                    "<init>",
                    symbol.getParameterTypes()
                );
            default:
                return false;
        }
    }

    private static boolean isDeprecatedMethod(
        Map<String, Boolean> deprecatedMethods,
        String methodName,
        List<String> parameterTypes
    ) {
        String exactKey = methodName + "(" + String.join(",", parameterTypes) + ")";
        if (deprecatedMethods.containsKey(exactKey)) {
            return true;
        }
        if (!hasTypeVariable(parameterTypes)) {
            return false;
        }

        for (String candidateKey : deprecatedMethods.keySet()) {
            int openParen = candidateKey.indexOf('(');
            int closeParen = candidateKey.lastIndexOf(')');
            if (openParen <= 0 || closeParen <= openParen) {
                continue;
            }

            String candidateName = candidateKey.substring(0, openParen);
            if (!methodName.equals(candidateName)) {
                continue;
            }

            List<String> candidateParams = parseParamList(
                candidateKey.substring(openParen + 1, closeParen)
            );
            if (candidateParams.size() != parameterTypes.size()) {
                continue;
            }
            if (paramsCompatible(parameterTypes, candidateParams)) {
                return true;
            }
        }
        return false;
    }

    private static List<String> parseParamList(String raw) {
        if (raw == null || raw.isEmpty()) {
            return Collections.emptyList();
        }
        String[] split = raw.split(",");
        List<String> params = new ArrayList<String>(split.length);
        for (String value : split) {
            params.add(value == null ? "" : value.trim());
        }
        return params;
    }

    private static boolean paramsCompatible(
        List<String> observedParams,
        List<String> candidateParams
    ) {
        for (int i = 0; i < observedParams.size(); i++) {
            String observed = observedParams.get(i);
            String candidate = candidateParams.get(i);
            if (observed.equals(candidate)) {
                continue;
            }
            if (isTypeVariableName(observed)) {
                continue;
            }
            return false;
        }
        return true;
    }

    private static boolean hasTypeVariable(List<String> params) {
        for (String param : params) {
            if (isTypeVariableName(param)) {
                return true;
            }
        }
        return false;
    }

    private static boolean isTypeVariableName(String value) {
        if (value == null || value.isEmpty()) {
            return false;
        }
        String trimmed = value.trim();
        while (trimmed.endsWith("[]")) {
            trimmed = trimmed.substring(0, trimmed.length() - 2);
        }
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

    Set<String> allDeprecatedSymbols() throws IOException {
        Set<String> deprecated = new LinkedHashSet<String>();
        List<String> classNames = new ArrayList<String>(classLocations.keySet());
        Collections.sort(classNames);

        for (String className : classNames) {
            ClassDeprecationInfo info = loadClassInfo(className);
            if (info == null) {
                continue;
            }
            if (info.classDeprecated) {
                deprecated.add(className);
            }

            List<String> fields = new ArrayList<String>(info.deprecatedFields.keySet());
            Collections.sort(fields);
            for (String field : fields) {
                deprecated.add(className + "#" + field);
            }

            List<String> methods = new ArrayList<String>(info.deprecatedMethods.keySet());
            Collections.sort(methods);
            for (String method : methods) {
                deprecated.add(className + "#" + method);
            }
        }

        return deprecated;
    }

    private void indexClassLocations(List<Path> jarPaths) throws IOException {
        for (Path jarPath : jarPaths) {
            if (jarPath == null) {
                continue;
            }
            try (JarFile jarFile = new JarFile(jarPath.toFile())) {
                java.util.Enumeration<JarEntry> entries = jarFile.entries();
                while (entries.hasMoreElements()) {
                    JarEntry entry = entries.nextElement();
                    if (entry.isDirectory()) {
                        continue;
                    }
                    String name = entry.getName();
                    if (!name.endsWith(".class")) {
                        continue;
                    }
                    String className = name
                        .substring(0, name.length() - ".class".length())
                        .replace('/', '.')
                        .replace('$', '.');
                    if (!classLocations.containsKey(className)) {
                        classLocations.put(className, new ClassLocation(jarPath, name));
                    }
                }
            } catch (IOException ignored) {
                // Unreadable jars are ignored for deprecation lookup.
            }
        }
    }

    private ClassDeprecationInfo loadClassInfo(String className) throws IOException {
        if (className == null || className.isEmpty()) {
            return null;
        }
        ClassDeprecationInfo cached = classInfoByName.get(className);
        if (cached != null) {
            return cached;
        }

        ClassLocation location = classLocations.get(className);
        if (location == null) {
            return null;
        }

        ClassDeprecationInfo info = new ClassDeprecationInfo();
        try (JarFile jarFile = new JarFile(location.jarPath.toFile())) {
            JarEntry entry = jarFile.getJarEntry(location.entryName);
            if (entry == null) {
                return null;
            }
            try (InputStream input = jarFile.getInputStream(entry)) {
                ClassReader reader = new ClassReader(input);
                reader.accept(new DeprecatedClassVisitor(info), ClassReader.SKIP_DEBUG);
            }
        } catch (IOException ignored) {
            return null;
        }

        classInfoByName.put(className, info);
        return info;
    }

    private static String methodKey(String name, String descriptor) {
        Type[] args = Type.getArgumentTypes(descriptor);
        List<String> params = new ArrayList<String>(args.length);
        for (Type arg : args) {
            params.add(typeName(arg));
        }
        return name + "(" + String.join(",", params) + ")";
    }

    private static String typeName(Type type) {
        switch (type.getSort()) {
            case Type.BOOLEAN:
                return "boolean";
            case Type.BYTE:
                return "byte";
            case Type.CHAR:
                return "char";
            case Type.DOUBLE:
                return "double";
            case Type.FLOAT:
                return "float";
            case Type.INT:
                return "int";
            case Type.LONG:
                return "long";
            case Type.SHORT:
                return "short";
            case Type.VOID:
                return "void";
            case Type.ARRAY:
                return typeName(type.getElementType()) + arraySuffix(type.getDimensions());
            case Type.OBJECT:
            default:
                return type.getClassName().replace('$', '.');
        }
    }

    private static String arraySuffix(int dimensions) {
        if (dimensions <= 0) {
            return "";
        }
        StringBuilder out = new StringBuilder(dimensions * 2);
        for (int i = 0; i < dimensions; i++) {
            out.append("[]");
        }
        return out.toString();
    }

    private static boolean isDeprecatedAccess(int access) {
        return (access & Opcodes.ACC_DEPRECATED) != 0;
    }

    private static final class ClassLocation {
        private final Path jarPath;
        private final String entryName;

        private ClassLocation(Path jarPath, String entryName) {
            this.jarPath = jarPath;
            this.entryName = entryName;
        }
    }

    private static final class ClassDeprecationInfo {
        private boolean classDeprecated;
        private final Map<String, Boolean> deprecatedFields =
            new HashMap<String, Boolean>();
        private final Map<String, Boolean> deprecatedMethods =
            new HashMap<String, Boolean>();
    }

    private static final class DeprecatedClassVisitor extends ClassVisitor {
        private final ClassDeprecationInfo target;

        private DeprecatedClassVisitor(ClassDeprecationInfo target) {
            super(Opcodes.ASM9);
            this.target = target;
        }

        @Override
        public void visit(
            int version,
            int access,
            String name,
            String signature,
            String superName,
            String[] interfaces
        ) {
            if (isDeprecatedAccess(access)) {
                target.classDeprecated = true;
            }
        }

        @Override
        public AnnotationVisitor visitAnnotation(String descriptor, boolean visible) {
            if (DEPRECATED_DESCRIPTOR.equals(descriptor)) {
                target.classDeprecated = true;
            }
            return super.visitAnnotation(descriptor, visible);
        }

        @Override
        public FieldVisitor visitField(
            int access,
            String name,
            String descriptor,
            String signature,
            Object value
        ) {
            final boolean[] deprecated = new boolean[] { isDeprecatedAccess(access) };
            return new FieldVisitor(Opcodes.ASM9) {
                @Override
                public AnnotationVisitor visitAnnotation(
                    String annotationDescriptor,
                    boolean visible
                ) {
                    if (DEPRECATED_DESCRIPTOR.equals(annotationDescriptor)) {
                        deprecated[0] = true;
                    }
                    return super.visitAnnotation(annotationDescriptor, visible);
                }

                @Override
                public void visitEnd() {
                    if (deprecated[0]) {
                        target.deprecatedFields.put(name, Boolean.TRUE);
                    }
                }
            };
        }

        @Override
        public MethodVisitor visitMethod(
            int access,
            String name,
            String descriptor,
            String signature,
            String[] exceptions
        ) {
            String key = methodKey(name, descriptor);
            final boolean[] deprecated = new boolean[] { isDeprecatedAccess(access) };
            return new MethodVisitor(Opcodes.ASM9) {
                @Override
                public AnnotationVisitor visitAnnotation(
                    String annotationDescriptor,
                    boolean visible
                ) {
                    if (DEPRECATED_DESCRIPTOR.equals(annotationDescriptor)) {
                        deprecated[0] = true;
                    }
                    return super.visitAnnotation(annotationDescriptor, visible);
                }

                @Override
                public void visitEnd() {
                    if (deprecated[0]) {
                        target.deprecatedMethods.put(key, Boolean.TRUE);
                    }
                }
            };
        }
    }
}
