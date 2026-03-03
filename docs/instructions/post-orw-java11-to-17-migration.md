# Post‑OpenRewrite Java 11 → 17 Migration Guide (Code Changes Only)

This document defines what a **code agent** must do **after** running the OpenRewrite recipe  
`org.openrewrite.java.migrate.UpgradeToJava17` on a Java 11 codebase.

- **Scope**: only edit code, build descriptors, and version‑controlled configuration files.
- **Forbidden** while following this guide:
  - Do **not** run any builds, tests, linters, or IDE refactorings.
  - Do **not** change CI images, Docker base images, OS packages, or JDK installations.

The goal is a Java 17–ready codebase that compiles and runs *once someone else* updates the
environment and executes builds/tests.

---

## 0. Preconditions and What OpenRewrite Already Did

Assume the OpenRewrite recipe `org.openrewrite.java.migrate.UpgradeToJava17` has completed successfully.
That recipe already:

- Upgraded the Java **language level** in build files
  - Switched Maven/Gradle configuration to target Java 17 (`UpgradeBuildToJava17` / `UpgradeJavaVersion`).
  - Upgraded key Maven/Gradle plugins to Java‑17‑compatible ranges
    (compiler, war, checkstyle, SpotBugs, some GitHub Actions and wrapper versions).
- Applied specific JDK API migrations
  - Removed `java.lang.Runtime.traceInstructions(boolean)` and `traceMethodCalls(boolean)`.
  - Converted `javax.tools.ToolProvider` and some `java.lang.reflect.Modifier` /
    `java.lang.invoke.ConstantBootstraps` instance calls to static calls.
  - Adjusted deprecated certificate/logging APIs and SunJSSE provider names.
  - Ensured Java agent `premain`/`agentmain` methods are `public`.
  - Replaced removed `finalize()` methods on some JDK types
    (`java.util.zip.*`, `java.io.FileInputStream` / `FileOutputStream`) with the supported methods.
  - Replaced `SSLSession.getPeerCertificateChain()` with `getPeerCertificates()`.
  - Repointed `com.sun.net.ssl` usage to `javax.net.ssl`.
- Modernized some language usage
  - Introduced `instanceof` pattern matching where safe.
  - Introduced `String#formatted(...)` and some text blocks (`"""..."""`) where configured.
- Updated some dependency versions
  - Upgraded **Guice**, **commons‑codec**, and **MapStruct** to Java‑17‑ready versions.
  - Added `lombok-mapstruct-binding` where Lombok + MapStruct are used.

**Do not** re‑implement or undo any of the above. Treat them as **completed**.  
The remaining steps focus on gaps that OpenRewrite does **not** cover.

### 0.1 Centralized Java/Kotlin target configuration (build descriptors only)

OpenRewrite upgrades *typical* build settings, but projects often centralize Java/Kotlin
targets in shared constants (for example Gradle version catalogs).

Actions:

- Search build descriptors and version catalogs for shared JVM target values:
  - `gradle/libs.versions.toml`
  - `gradle.properties`
  - `buildSrc` or convention plugins
  - Any `jvmTarget`, `javaVersion`, `javaLanguageVersion`, or similar keys.
- In Gradle build files, replace JavaVersion short constants:
  - `= VERSION_11` → `= VERSION_17`
- For each constant that is wired into **both**:
  - `JavaPluginExtension` (`sourceCompatibility`, `targetCompatibility`), and
  - Kotlin compile tasks (`kotlinOptions.jvmTarget` or `jvmToolchain`),
  ensure the value represents Java 17.

**Example (Gradle version catalog)**

Before (`gradle/libs.versions.toml`):

```toml
[versions]
jvmTarget = "11"
```

After:

```toml
[versions]
jvmTarget = "17"
```

Do not change numbers that are clearly *not* Java versions (for example detekt thresholds,
business constants, or test data).

### 0.2 Stale Java‑version markers in build/config files

Without running any builds, you can still clean up obviously stale Java‑version references.

Actions:

- Text‑search build and configuration files for:
  - `Java 11`
  - `JDK 11`
  - `1.8`
  - `jdk8`
- For each hit:
  - If it documents the project’s required Java version, update to `Java 17` / `JDK 17`.
  - If it is test data or an unrelated constant, leave it unchanged.
  - If you are unsure, add a brief comment:

    ```text
    # TODO: Verify whether this Java version reference should be updated for Java 17.
    ```

### 0.3 Kotlin plugin version awareness (no automatic upgrade)

Kotlin Gradle/Maven plugins older than the Java 17 toolchain can work but may be poorly tested
for `jvmTarget = "17"`. This guide does **not** mandate upgrading them automatically, but the
agent should surface obvious mismatches for humans.

Actions:

- Search build descriptors for Kotlin plugin declarations:
  - Gradle Kotlin DSL:

    ```kotlin
    plugins {
        kotlin("jvm") version "1.6.10"
    }
    ```

  - Gradle Groovy DSL:

    ```groovy
    plugins {
        id "org.jetbrains.kotlin.jvm" version "1.6.10"
    }
    ```

  - Maven:

    ```xml
    <plugin>
      <groupId>org.jetbrains.kotlin</groupId>
      <artifactId>kotlin-maven-plugin</artifactId>
      <version>1.6.10</version>
    </plugin>
    ```

- If the project’s Java/Kotlin target is 17 (toolchain, `sourceCompatibility`, `kotlinOptions.jvmTarget`)
  and the Kotlin plugin version is clearly old (for example `1.6.x` or earlier), add a nearby comment:

  ```kotlin
  // TODO: Kotlin plugin version is older than the Java 17 target; 
  // consider upgrading per Kotlin's official compatibility matrix.
  ```

Do **not** bump the Kotlin plugin version automatically in this guide; only annotate the mismatch.

---

## 1. Clean Up Remaining JDK‑Internal API Usage

OpenRewrite handles only a subset of internal APIs (for example `com.sun.net.ssl`).  
You must eliminate all *other* internal dependencies from project code and configs.

### 1.1 Search targets

Search in `src/`, `test/`, and configuration files:

- `import sun.`
- `import com.sun.`
- `jdk.internal.`
- `--add-opens`
- `--add-exports`

For each match, classify and fix as below.

### 1.2 Replace `sun.misc.BASE64Encoder` / `BASE64Decoder`

**Before**

```java
import sun.misc.BASE64Encoder;
import sun.misc.BASE64Decoder;

String encoded = new BASE64Encoder().encode(bytes);
byte[] decoded = new BASE64Decoder().decodeBuffer(encoded);
```

**After**

```java
import java.util.Base64;

String encoded = Base64.getEncoder().encodeToString(bytes);
byte[] decoded = Base64.getDecoder().decode(encoded);
```

Actions:
- Remove imports from `sun.misc.*`.
- Add `java.util.Base64` import.

### 1.3 Replace `sun.misc.Unsafe` usage

Search for `Unsafe`:

- If it is only used for CAS / atomic operations, replace with `java.util.concurrent` types.

**Before**

```java
private static final Unsafe UNSAFE = ...;
private static final long VALUE_OFFSET = ...;

void increment() {
    int v;
    do {
        v = UNSAFE.getIntVolatile(this, VALUE_OFFSET);
    } while (!UNSAFE.compareAndSwapInt(this, VALUE_OFFSET, v, v + 1));
}
```

**After**

```java
import java.util.concurrent.atomic.AtomicInteger;

private final AtomicInteger value = new AtomicInteger();

void increment() {
    value.incrementAndGet();
}
```

If `Unsafe` is used for off‑heap memory or object construction without constructors,
add `// TODO` comments and flag for human review, since a redesign is usually required.

### 1.4 Replace other `com.sun.*` usages (except `com.sun.net.ssl`)

For each `import com.sun.*` that is *not* in the SSL package:

- Identify the supported replacement (often a public `java.*`, `javax.*`, or `jakarta.*` API).
- Replace the import and adapt the call sites.

Example: `com.sun.org.apache.xerces.internal` → a standard XML library:

```java
// Before
import com.sun.org.apache.xerces.internal.jaxp.SAXParserFactoryImpl;

SAXParserFactory factory = new SAXParserFactoryImpl();
```

```java
// After
import javax.xml.parsers.SAXParserFactory;

SAXParserFactory factory = SAXParserFactory.newInstance();
```

If no supported replacement exists, add:

```java
// TODO: Uses JDK-internal API <fully-qualified-name>; requires design decision on replacement.
```

and leave a concise note for human review.

---

## 2. Remove Fragile Reflection on JDK Internals

OpenRewrite does not remove all problematic reflective calls.  
You must minimize reflection that depends on JDK internals or module‑encapsulated members.

### 2.1 Search targets

- `setAccessible(true`
- `Class.forName("java.``
- `Class.forName("sun.``
- `getDeclaredField("` or `getDeclaredMethod("` on JDK types.

### 2.2 Replace reflective access with public APIs

Typical pattern:

**Before**

```java
Field f = SomeJdkClass.class.getDeclaredField("someField");
f.setAccessible(true);
Object value = f.get(instance);
```

**After (preferred)**

```java
// Expose a real API instead of reflective access
public class SomeWrapper {
    private final SomeJdkClass delegate;

    public SomeWrapper(SomeJdkClass delegate) {
        this.delegate = delegate;
    }

    public Object getSomeField() {
        return delegate.getSomeField(); // use official getter or behavior
    }
}
```

Actions:
- If reflection targets **project types**, introduce proper methods/constructors and call them directly.
- If reflection targets **JDK types**, either:
  - Replace with supported methods, or
  - Add a `// TODO` comment describing the dependency and flag for human review.

Do not introduce new `--add-opens` or `--add-exports` options in code or configs.

---

## 3. Security Manager and Legacy Sandbox Logic

Java 17 deprecates the `SecurityManager` for removal. OpenRewrite does not remove custom usage.

### 3.1 Search targets

- `System.setSecurityManager(`
- `System.getSecurityManager(`
- `extends SecurityManager`
- `java.lang.SecurityManager`
- Security policy files referenced from code (e.g. `"java.security.policy"` system properties).

### 3.2 Remove or isolate security‑manager code

**Before**

```java
public static void main(String[] args) {
    System.setSecurityManager(new SecurityManager());
    runSandboxed(args);
}
```

**After**

```java
public static void main(String[] args) {
    // TODO: SecurityManager is deprecated/removed on Java 17+.
    // Replace with a different sandbox or process-level isolation.
    runSandboxed(args);
}
```

Actions:
- Remove explicit `System.setSecurityManager(...)` calls.
- Remove application‑specific `SecurityManager` subclasses if they are no longer used.
- Replace checks that depended on `checkPermission` with:
  - Application‑level authorization / role checks, or
  - OS / container isolation (documented via `// TODO` for humans to implement).

Keep changes minimal: delete or comment out code and leave short, clear `TODO` notes instead of fully redesigning security in an automated fashion.

---

## 4. Nashorn and Other Removed JDK Modules

Nashorn JavaScript engine and several Java EE modules are gone by Java 17.  
OpenRewrite does **not** replace application‑level usage of these APIs.

### 4.1 Nashorn removal

Search:

- `jdk.nashorn.api.scripting`
- `getEngineByName("nashorn")`
- Scripts or helpers referencing `"nashorn"` explicitly.

**Before**

```java
import javax.script.ScriptEngine;
import javax.script.ScriptEngineManager;

ScriptEngine engine = new ScriptEngineManager().getEngineByName("nashorn");
Object result = engine.eval(jsCode);
```

**After (placeholder)**

```java
// TODO: Nashorn removed in Java 17.
// Replace with a supported JS engine (for example, GraalJS) or remove scripting.
throw new UnsupportedOperationException("JavaScript execution engine must be replaced for Java 17+");
```

Actions:
- Remove Nashorn imports.
- Replace Nashorn setup with a clear `UnsupportedOperationException` and `TODO` comment,
  or factor out to an interface that can later be backed by a new JS engine.

### 4.2 Java EE modules removed from JDK

If the code assumed Java EE APIs from the JDK (JAX‑WS, JAXB, etc.) instead of explicit dependencies:

- Ensure all such imports come from standalone artifacts (`jakarta.*` or `javax.*` from dependencies),
  not from JDK modules.
- If code refers to packages that no longer exist in JDK 17 without external libs, add `// TODO`
  noting that an explicit dependency must be added externally.

Example: JAXB

```java
// OK: project is already on standalone JAXB
import jakarta.xml.bind.JAXBContext;
```

If you still see `javax.xml.bind` imports without a corresponding library, add:

```java
// TODO: Requires standalone JAXB dependency; add via build tooling.
```

and keep the code otherwise unchanged.

---

## 5. Jakarta Migration in Application Code (javax.* → jakarta.*)

OpenRewrite’s Java 17 recipe does **not** perform full Jakarta EE migration.  
If your stack has already moved to Jakarta‑based versions (Spring 6+, Jakarta EE 9+, Spring Boot 3+),
you must align imports and code.

### 5.1 When to apply this section

Apply these steps **only if**:

- The project depends on Jakarta‑based libraries (for example Spring 6+, Jakarta EE 9+ / 10+),
  **and**
- Old `javax.*` imports still appear in your code.

If the project is still on Java‑11‑era libraries that require `javax.*`, **do not** rewrite imports here;
flag for human review instead.

### 5.2 Systematic import migration

Search for:

- `javax.servlet.`
- `javax.persistence.`
- `javax.validation.`
- `javax.inject.`

Replace imports and types according to the library baseline.

**Example: Servlet API**

```java
// Before
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
```

```java
// After (Jakarta)
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
```

**Example: JPA entities**

```java
// Before
import javax.persistence.Entity;
import javax.persistence.Id;
```

```java
// After
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
```

Actions:
- Update import statements.
- Ensure annotation names remain the same (only the package usually changes).
- Do not change business logic or field names.

If a `javax.*` type has no Jakarta equivalent in the chosen framework version, add:

```java
// TODO: javax.* type <TypeName> has no direct Jakarta equivalent in current stack.
```

and leave it for manual design.

---

## 6. Logging Stack Modernization

Java 17 itself does not mandate logging changes, but older logging stacks often block upgrades.
This section is complementary to OpenRewrite (which does **not** remove Log4j 1.x).

### 6.1 Eliminate Log4j 1.x from application code

Search:

- `import org.apache.log4j.`
- `Logger.getLogger(`
- `PropertyConfigurator.configure(`
- `log4j.xml` / `log4j.properties`.

**Before**

```java
import org.apache.log4j.Logger;

public class OrderService {
    private static final Logger log = Logger.getLogger(OrderService.class);

    public void process(Order order) {
        log.info("Processing " + order.getId());
    }
}
```

**After (SLF4J API + Logback/Log4j2 backend)**

```java
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

public class OrderService {
    private static final Logger log = LoggerFactory.getLogger(OrderService.class);

    public void process(Order order) {
        log.info("Processing {}", order.getId());
    }
}
```

Actions:
- Replace `org.apache.log4j.Logger` imports with `org.slf4j.Logger` and `LoggerFactory`.
- Replace concatenated logging with parameterized logging where trivial.
- Delete any remaining references to Log4j 1.x appenders/layouts in code.
- Leave configuration file migration (XML/properties) as TODO comments if required.

### 6.2 Replace Commons Logging direct usage

Search:

- `org.apache.commons.logging.Log`
- `LogFactory.getLog(`

**Before**

```java
import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;

private static final Log log = LogFactory.getLog(MyClass.class);
```

**After**

```java
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

private static final Logger log = LoggerFactory.getLogger(MyClass.class);
```

Update call sites from `log.debug(...)` / `log.info(...)` to SLF4J equivalents (method names are the same).

---

## 7. Test Code Migration (Java 11 → 17, Code‑Only)

You must not *run* tests, but you may refactor test **code** to be Java‑17‑ready.

### 7.1 Migrate JUnit 3/4 tests to JUnit 5 style

This guide does not assume any OpenRewrite JUnit recipes have run.

Search:

- `extends TestCase`
- `org.junit.Test`
- `@RunWith(`
- `@Rule` / `@ClassRule`
- `@Before` / `@After` / `@BeforeClass` / `@AfterClass`

**Example migration**

```java
// Before (JUnit 4)
import org.junit.Test;
import org.junit.Before;

public class CalculatorTest {
    private Calculator calculator;

    @Before
    public void setUp() {
        calculator = new Calculator();
    }

    @Test(expected = IllegalArgumentException.class)
    public void dividesByZero() {
        calculator.divide(1, 0);
    }
}
```

```java
// After (JUnit 5)
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.assertThrows;

class CalculatorTest {
    private Calculator calculator;

    @BeforeEach
    void setUp() {
        calculator = new Calculator();
    }

    @Test
    void dividesByZero() {
        assertThrows(IllegalArgumentException.class,
                () -> calculator.divide(1, 0));
    }
}
```

Actions:
- Move to `org.junit.jupiter.*` imports and annotations.
- Use `assertThrows` instead of `@Test(expected = ...)`.
- Remove JUnit 4 runners (`@RunWith`) where possible and replace with JUnit 5 extensions.

Do **not** add or change build plugins for test execution in this guide.

### 7.2 Mockito / PowerMock and Java 17

Search:

- `org.powermock` imports.
- Deprecated Mockito constructs or inline‑mocking hacks.

Actions:
- Where PowerMock is only used to mock static methods or constructors, prefer extracting interfaces
  or introducing seam interfaces, leaving `// TODO` where a design change is required.
- Keep transformations conservative; avoid adding new test libraries here.

---

## 8. JVM Flags and Launch Scripts in Code Repositories

This section only covers **text changes** in version‑controlled scripts and configs,
not environment provisioning.

### 8.1 Remove obsolete/forbidden flags

Search in shell scripts, `.bat` files, YAML, and properties under version control:

- `--illegal-access`
- `--add-opens`
- `--add-exports`
- `--add-modules java.se.ee`
- `-Xverify:none`

For each occurrence:

- If the flag enables access to JDK internals that you removed in sections 1–2, delete the flag.
- If deleting the flag would hide a still‑needed internal access, leave:

```bash
# TODO: This JVM flag was used to open <module/package>; requires manual review on Java 17.
```

and keep the flag for human decision.

### 8.2 Keep runtime configuration minimal

Avoid introducing new flags or tuning options in this guide.  
Your role is to **simplify** and annotate, not to re‑tune the JVM.

---

## 9. Optional Java 17+ Refactorings (Not Covered by OpenRewrite)

The Java 17 recipe already uses some modern constructs (text blocks, `String#formatted`, some pattern matching).  
This section focuses on complementary refactorings that the recipe does **not** perform automatically.

Only apply these when the transformation is straightforward and does not change behavior.

### 9.1 Replace boilerplate DTOs with records

Search for simple, immutable data holders:

- `final` fields only.
- Constructors that just assign parameters.
- `equals`, `hashCode`, `toString` that match field structure.

**Before**

```java
public final class User {
    private final String name;
    private final int age;

    public User(String name, int age) {
        this.name = name;
        this.age = age;
    }

    public String getName() {
        return name;
    }

    public int getAge() {
        return age;
    }
}
```

**After**

```java
public record User(String name, int age) { }
```

Actions:
- Ensure the class has no additional behavior tied to object identity (such as non‑trivial inheritance).
- Convert only clearly immutable types.

### 9.2 Use `switch` expressions where safe

OpenRewrite’s Java 17 recipe does not introduce `switch` expressions by default.

**Before**

```java
int score(Color color) {
    switch (color) {
        case RED:
            return 1;
        case GREEN:
            return 2;
        default:
            return 0;
    }
}
```

**After**

```java
int score(Color color) {
    return switch (color) {
        case RED -> 1;
        case GREEN -> 2;
        default -> 0;
    };
}
```

Actions:
- Only convert switches that are pure and side‑effect‑free.

### 9.3 Improve `instanceof` chains with pattern matching (complementary)

OpenRewrite applies `InstanceOfPatternMatch` in many cases, but it may skip complex ones.

Search for:

- `if (x instanceof SomeType)` followed by a cast of `x`.

**Before**

```java
if (obj instanceof String) {
    String s = (String) obj;
    handle(s);
}
```

**After**

```java
if (obj instanceof String s) {
    handle(s);
}
```

Skip cases where OpenRewrite has already updated the pattern.

---

## 10. Code‑Centric Search Checklist (No Builds / Tests)

Use text search only; do not execute any commands that compile or run the code.

### 10.1 High‑risk APIs and patterns

- `import sun.`
- `import com.sun.`
- `jdk.internal.`
- `setAccessible(`
- `SecurityManager`
- `finalize(`
- `jdk.nashorn`
- `getEngineByName("nashorn")`
- `javax.servlet.`
- `javax.persistence.`
- `javax.validation.`
- `javax.inject.`
- `org.apache.log4j.`
- `extends TestCase`
- `@RunWith(`
- `Thread.countStackFrames(`
- `SSLSession.getPeerCertificateChain(`

### 10.2 Scripts and configuration

- `--illegal-access`
- `--add-opens`
- `--add-exports`
- `java.se.ee`
- `-Xverify:none`

For each hit, apply the relevant section above or leave a minimal `// TODO` / comment when a human decision is required.

---

## 11. Official Reference Documentation

Use these links for precise semantics and migration details. Prefer them over blogs or Q&A sites.

- **Java 11**
  - JDK 11 documentation home (Oracle):  
    `https://docs.oracle.com/javase/11/`
  - JDK 11 Migration Guide index (Oracle “Books” page, includes Migration Guide):  
    `https://docs.oracle.com/en/java/javase/11/books.html`
- **Java 17**
  - JDK 17 documentation home (Oracle):  
    `https://docs.oracle.com/en/java/javase/17/`
  - JDK 17 API “New since JDK 11” overview (helps understand APIs newly available when moving 11 → 17):  
    `https://docs.oracle.com/en/java/javase/17/docs/api/new-list.html`
- **Java Version History and LTS Context**
  - Java SE 11 and 17 overview, including LTS status and JEP references:  
    `https://en.wikipedia.org/wiki/Java_version_history`
- **Kotlin**
  - Kotlin FAQ (includes supported JVM versions and general language info):  
    `https://kotlinlang.org/docs/faq.html`
  - Java ↔ Kotlin interoperability details (nullability, mapped types, generics):  
    `https://kotlinlang.org/docs/java-interop.html`

When library‑ or framework‑specific questions arise (for example, Spring 6 + Java 17, Jakarta EE 10),
always prefer:

- The framework’s **official reference docs** and **migration guides**.
- The library’s **release notes** and **GitHub issues**.

This guide deliberately avoids duplicating what the OpenRewrite `UpgradeToJava17` recipe
and official migration guides already do, and focuses instead on manual code changes that
automated tools cannot safely complete.
