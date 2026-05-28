# TODO

## CLI Run machinery refactor

- update `ploy run ...`

```bash
ploy run <spec-path> [ <repo-path> = . {--apply} | <namespace/repo{:<sha|branch>}> ] { --pull <artifacts-path> }
    # run spec for a [ local repo { and apply result } or remote repo ] and put final artifacts
    # spec is a folder that contains mig.yaml or full path to any yaml-file that is compliant with spec
    # if no <repo-path> / <namespace/repo> is specified, then ploy looks into CWD
    # if at CWD is not a git repo -> hard fail; otherwise, convert from HEAD to <sha:namespace/repo>, no diff/staged included
    # if --apply then apply resulting patch if succeeded
    # remove other flags and corresponding machinery (--json, --job*, --cap*, --max-retries, --target-ref)
ploy run ls {[ | <path> | <sha|branch>:namespace/repo ]} {--per-page X} {--page Y} # list last runs { for a specific repo }
ploy run cancel <run-id>
ploy run apply <run-id> {<path> = .} # apply patch from run-id to the repo in <path>, defaulting <path> to CWD; replaces `ploy run patch`
ploy run pull <run-id> {<artifacts-path>} # pull artifacts to a specific path; if not specified - put in OS-generated tmp
```

- remove `ploy run patch` (replaced with `ploy run apply`);
- remove previous machinery for `ploy run pull`;
- remove `ploy run start` entierly;
- keep `ploy run status`.

## Specs

- remove `kind` field
- add `title` field for optional title
- add `slug` field for optional spec slug name without spaces

## Users

1. Rename role `cli-admin` to `admin`
2. Rename role `control-plane` to `user`
3. Replace `description` with `username`, mandatory for token create/revoke
4. Token + Active + Username must be uniq

```bash
ploy cluster token create <username> <role>
ploy cluster token revoke <username>
```

## OpenRewrite Support

1. Provide avail community ORW recipes via `ploy orw ls`

2. Autofill coords (envs) for the ORW image by special prop 'orw'.

For example,

```yaml
  orw: java.spring.boot3.UpgradeSpringBoot_3_0
```

adds on the execution time

```yaml
  envs:
    RECIPE_GROUP: org.openrewrite.recipe
    RECIPE_ARTIFACT: rewrite-spring
    RECIPE_CLASSNAME: org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0
    RECIPE_VERSION: "6.28.2"
    MAVEN_PLUGIN_VERSION: "6.35.0"
    GRADLE_PLUGIN_VERSION: "7.29.0"
```
