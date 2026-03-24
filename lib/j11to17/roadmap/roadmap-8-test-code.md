# 8 Test code migration

## Actions
1. Replace JUnit 3/4 APIs with JUnit 5 (`org.junit.jupiter.api.*`) in test source files.
2. Replace `@Test(expected = ...)` with `assertThrows(...)` blocks.
3. Replace runner/rule patterns with JUnit 5 extension-style equivalents where straightforward.
4. For PowerMock-heavy cases that require design changes, keep test readable and add `TODO(java17): replace PowerMock usage in <test-class>`.
