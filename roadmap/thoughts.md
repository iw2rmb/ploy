Ok, i collected build logs, amata runs and diffs of run 3CAhnDZNSUr66d5mxmWUiThakpn into /Users/v.v.kovalev/@scale/ploy-lib/research/3CAhnDZNSUr66d5mxmWUiThakpn.

this is research material for the following idea how to raise efficiency of ploy heal process and significant tokens burn efficieny.

what do we have:
1. heal/ today has router that classifies error out of build log and then calls code, deps, or infra healing steps.
2. code/deps step fails to fit TPM cap and fails to comply search policy when it is required to make mass changes over code base (for example, replace one
function call to another)
3. part of this problem is that for code/deps step only summary provided, while build gate log can contain specific lines of code required to replace.
4. ploy trims the log, yet it remains oveloaded with irrelevant details sometimes, like ':compileJava' stacktrace for compiler errors that bring no value yet
can take 1/2..2/3 of the log.
5. also, on "mass issues" like new package changed api, multiple messages "symbol not found" are similar and only diverse in locations in files.
6. for mass editing, there are better tools for coding agent (tokens-burning speaking): openrewrite recipes and bash scripts. But context is required to make
proper decision.

How I want to transform that.
1. I want for trimmer to shrink log smarter (reduce size) and provide it structured, for it to be small enough to be safely transited to other steps.
2. I want for heal to use bash scripts or open rewrite declarative recipe (yaml) for mass editing.

## Trimmer
It should provide structured format errors.yaml:

1.1. default form
```yaml
errors:
    - error: # "What went wrong" content
      source: /workspace/build.gradle:8 # Where content Build file '/workspace/build.gradle' line: 8
      exception: # Exception stacktrace...
```

1.2. when it's a :compileJava error (`Execution failed for task ':compileJava'` in "What went wrong") drop compileJava stacktrace and provide:

```yaml
task: compileJava # org.gradle.api.tasks.TaskExecutionException: Execution failed for task ':compileJava'.
errors:
    - error: package does not exist
      package: io.swagger.v3.oas.annotations.media
      files:
        - path: /workspace/build/generated/sources/openapi/src/main/java/ru/tinkoff/capital/funds/api/model/GetIndexResponse.java:15
        - path: /workspace/build/generated/sources/openapi/src/main/java/ru/tinkoff/capital/funds/api/model/IndexFullInfo.java:20
        - path: /workspace/build/generated/sources/openapi/src/main/java/ru/tinkoff/capital/funds/api/model/Indicator.java:11
    ...
    - error: cannot find symbol # grouped by symbol when there's no location
      symbol: class Schema
      files:
        - path: /workspace/build/generated/sources/openapi/src/main/java/ru/tinkoff/capital/funds/api/model/Indicator.java:21
          snippet: @Schema(name = "Indicator", description = "Список показателей индекса")
          ...
    - error: cannot find symbol # grouped by symbol+location
      symbol: class IndexFullInfoFollowerFunds
      location: class GetIndexResponseBuilder
      files:
        - path: /workspace/src/main/java/ru/tinkoff/capital/funds/api/builder/operation/index/GetIndexResponseBuilder.java:137
          snippet: private List<IndexFullInfoFollowerFunds> buildFollowerFunds(List<FundRow> funds) {
          ...
    - error: some other error # errors that out of classification
      raw: # content after filepath/snippet
      path: 
      snippet:
...

1.3. when it's a plugin apply exception ("What went wrong" starts with "An exception occurred applying plugin request")
```yaml
errors:
    - error: # multiline content after "Failed to apply plugin ..."
      plugin: org.springframework.boot # id: value
      version: 3.0.5
```

## Router

Router must provide array of tasks instead of putting everything in one bug summary.

```yaml
tasks:
    - error_kind:
      bug_summary:
      items: [] # indexes in errors.yaml to copy to prompt
```

Per task execution must help with tokens burn reduction.

## OpenRewrite

There must be a focused image for agent to use it to run openrewrite recipe (similar to hook running openapi-generator-api)

## Prompt

Prompt in deps/code must include clear criterias to decide edit with bash script or ORW recipe.
