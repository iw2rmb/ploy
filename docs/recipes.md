# Recipes Catalog and Validation

This guide shows how to discover OpenRewrite recipes and how the API validates a `recipe_id` during transforms.

## Enabling the Catalog

- Server routes: set `PLOY_ENABLE_RECIPES_CATALOG=true` to expose:
  - `GET /v1/arf/recipes?query=&pack=&version=&limit=`
  - `GET /v1/arf/recipes/:id`
  - `POST /v1/arf/recipes/refresh`

- CLI toggle: set `PLOY_RECIPES_CATALOG=true` to make `ploy arf recipe list/search` consume the catalog endpoints.

## Listing and Searching Recipes (CLI)

- List:
  - `ploy arf recipe list` (table)
  - `PLOY_RECIPES_CATALOG=true ploy arf recipe list -o json`

- Search:
  - `ploy arf recipe search "remove unused"`

## Transform-Time Validation

`POST /v1/arf/transforms` validates `recipe_id` when a catalog is available.

- If the recipe is not found, the API returns `400` with suggestions:

```
{
  "error": "invalid recipe_id",
  "message": "Recipe not found in catalog",
  "recipe_id": "org.openrewrite.java.RemoveUnusedImport",
  "suggestions": [
    "org.openrewrite.java.RemoveUnusedImports"
  ]
}
```

- Clients can surface these suggestions to users or retry with the corrected `recipe_id`.

CLI behavior:
- With `PLOY_RECIPES_CATALOG=true`, `ploy arf recipe run <id>` preflights your `recipe_id`. If unknown, it prints “Did you mean:” with up to 5 candidates and exits without running.

## Refreshing the Catalog

Use `POST /v1/arf/recipes/refresh` to rebuild the catalog. By default the server indexes a pinned set of packs. Platform configuration can extend this list.
