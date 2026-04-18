import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

import java.util.Arrays;
import java.util.List;
import org.junit.jupiter.api.Test;

class DependencyBomResolverTest {
    @Test
    void normalizeBomCoordinateValidatesAndCanonicalizesInput() {
        class SuccessCase {
            final String name;
            final String input;
            final String expected;

            SuccessCase(String name, String input, String expected) {
                this.name = name;
                this.input = input;
                this.expected = expected;
            }
        }

        class FailureCase {
            final String name;
            final String input;

            FailureCase(String name, String input) {
                this.name = name;
                this.input = input;
            }
        }

        List<SuccessCase> successCases = Arrays.asList(
            new SuccessCase(
                "trims surrounding whitespace",
                " org.springframework.boot:spring-boot-dependencies@3.0.13 ",
                "org.springframework.boot:spring-boot-dependencies@3.0.13"
            ),
            new SuccessCase(
                "supports arbitrary bom coordinates",
                "com.fasterxml.jackson:jackson-bom@2.17.2",
                "com.fasterxml.jackson:jackson-bom@2.17.2"
            )
        );

        List<FailureCase> failureCases = Arrays.asList(
            new FailureCase("missing @", "org.springframework.boot:spring-boot-dependencies"),
            new FailureCase("missing version", "org.springframework.boot:spring-boot-dependencies@"),
            new FailureCase("missing artifact", "org.springframework.boot:@3.0.13"),
            new FailureCase("missing group", ":spring-boot-dependencies@3.0.13"),
            new FailureCase("extra colon", "a:b:c@1.0.0"),
            new FailureCase("double @", "a:b@1.0.0@2.0.0")
        );

        for (SuccessCase testCase : successCases) {
            assertEquals(
                testCase.expected,
                DependencyBomResolver.normalizeBomCoordinate(testCase.input),
                testCase.name
            );
        }

        for (FailureCase testCase : failureCases) {
            assertThrows(
                IllegalArgumentException.class,
                () -> DependencyBomResolver.normalizeBomCoordinate(testCase.input),
                testCase.name
            );
        }
    }
}
