package build

import (
    "testing"

    "github.com/stretchr/testify/require"
)

func TestParseBuildErrors_JavaFormats(t *testing.T) {
    raw := "src/Main.java:[12,5] cannot find symbol\n" +
        "src/utils/Helper.java:7 missing return\n" +
        "C:/project/Foo.java:42:13 incompatible types"

    errs := ParseBuildErrors("java", "maven", raw)
    require.GreaterOrEqual(t, len(errs), 2)

    require.Equal(t, "java", errs[0].Language)
    require.Equal(t, "maven", errs[0].Tool)
    require.Equal(t, "src/Main.java", errs[0].File)
    require.Equal(t, 12, errs[0].Line)
    require.Equal(t, 5, errs[0].Column)

    // Verify colon-format and windows-style paths are parsed
    var foundHelper bool
    for _, e := range errs {
        if e.File == "src/utils/Helper.java" && e.Line == 7 && e.Column == 0 {
            foundHelper = true
        }
    }
    require.True(t, foundHelper)
}

func TestParseBuildErrors_DefaultsAndUnknownLanguage(t *testing.T) {
    errs := ParseBuildErrors("java", "", "App.java:1 error")
    require.Len(t, errs, 1)
    require.Equal(t, "maven", errs[0].Tool)

    none := ParseBuildErrors("rust", "", "file.rs:1 error")
    require.Nil(t, none)
}
