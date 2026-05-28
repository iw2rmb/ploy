# TODO

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
