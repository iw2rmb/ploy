package io.iw2rmb.ploy.orwcli;

import org.junit.jupiter.api.Test;

import java.nio.file.Path;
import java.nio.file.PathMatcher;
import java.nio.file.Paths;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class RewriteCliMainExcludeMatcherTest {
    private static Path workspaceRoot() {
        return Paths.get(System.getProperty("user.dir"))
            .resolve("workspace-root")
            .toAbsolutePath()
            .normalize();
    }

    @Test
    void doubleStarPrefixMatchesRootAndNestedPaths() {
        Path workspace = workspaceRoot();
        List<PathMatcher> matchers = RewriteCliMain.compileExcludePathMatchers("**/build-client-swagger.gradle");

        assertTrue(
            RewriteCliMain.matchesAnyExclude(workspace, workspace.resolve("build-client-swagger.gradle"), matchers),
            "Expected root file to match **/file pattern"
        );
        assertTrue(
            RewriteCliMain.matchesAnyExclude(workspace, workspace.resolve("nested/build-client-swagger.gradle"), matchers),
            "Expected nested file to match **/file pattern"
        );
    }

    @Test
    void doubleStarSuffixPatternMatchesRootAndNestedExtensions() {
        Path workspace = workspaceRoot();
        List<PathMatcher> matchers = RewriteCliMain.compileExcludePathMatchers("**/*.proto");

        assertTrue(
            RewriteCliMain.matchesAnyExclude(workspace, workspace.resolve("schema.proto"), matchers),
            "Expected root extension match for **/*.proto"
        );
        assertTrue(
            RewriteCliMain.matchesAnyExclude(workspace, workspace.resolve("src/main/proto/schema.proto"), matchers),
            "Expected nested extension match for **/*.proto"
        );
    }

    @Test
    void explicitRelativePatternStillMatchesOnlyExactRelativePath() {
        Path workspace = workspaceRoot();
        List<PathMatcher> matchers = RewriteCliMain.compileExcludePathMatchers("src/main/groovy/generated-client.gradle");

        assertTrue(
            RewriteCliMain.matchesAnyExclude(
                workspace,
                workspace.resolve("src/main/groovy/generated-client.gradle"),
                matchers
            ),
            "Expected exact relative path to match"
        );
        assertFalse(
            RewriteCliMain.matchesAnyExclude(workspace, workspace.resolve("generated-client.gradle"), matchers),
            "Did not expect root file to match explicit relative path"
        );
    }
}
