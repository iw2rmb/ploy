# 6 Jakarta migration gate and import rewrite

## Actions
1. Decide migration gate from dependency files first.
2. If baseline is Jakarta-first, replace matching `javax.*` imports with `jakarta.*` in source files.
3. If baseline is not Jakarta-first, keep `javax.*` and add `TODO(java17): blocked Jakarta rewrite by dependency baseline` near affected imports/classes.
4. Keep code logic and annotations unchanged; only package imports/types change.
