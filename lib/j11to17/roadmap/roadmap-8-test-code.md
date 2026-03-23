# 8 Test code migration

Parent: `roadmap.md` item `1.8`.
Source: `../post-orw-java11-to-17-migration.md` section `7`.

## Goal
Modernize test code to Java-17-friendly patterns without changing CI/runtime setup.

## Detailed actions
1. Migrate JUnit 3/4 imports and annotations to JUnit 5 (`org.junit.jupiter.*`).
2. Replace `@Test(expected=...)` with `assertThrows`.
3. Replace `@RunWith`, legacy rules, and base-class patterns where direct JUnit 5 equivalents exist.
4. Reduce PowerMock reliance via seams/interfaces where low-risk; mark hard cases with TODO.

## Before/after examples

```java
// Before
@Test(expected = IllegalArgumentException.class)
public void dividesByZero() {
    calculator.divide(1, 0);
}

// After
@Test
void dividesByZero() {
    assertThrows(IllegalArgumentException.class, () -> calculator.divide(1, 0));
}
```

## Verification checklist
- No `extends TestCase` or JUnit4 `@RunWith` remain in migrated scope.
- Remaining PowerMock dependencies are explicitly tracked with redesign TODOs.

## Sizing
- CFP_delta: 12
- Base reasoning: high
- Shifted for assumption-bound: xhigh
