# 9 JVM flags in scripts/config

Parent item: `roadmap.md` -> `1.9`.

## Edit targets
- launch scripts: `*.sh`, `*.bash`, `*.zsh`, `*.bat`, `*.cmd`
- runtime config files: `*.properties`, `*.yaml`, `*.yml`, `*.env`
- deployment descriptors in repo that embed JVM args

## Match strings
- `--illegal-access`
- `--add-opens`
- `--add-exports`
- `--add-modules java.se.ee`
- `-Xverify:none`

## Actions
1. Remove obsolete flags that were only needed for Java 8/11 compatibility.
2. Keep a risky flag only when the corresponding code path still depends on it.
3. For retained risky flags, add `TODO(java17): remove flag after replacing dependency on <module/package>` on the same file block.
4. Do not add new JVM compatibility flags in this migration slice.
