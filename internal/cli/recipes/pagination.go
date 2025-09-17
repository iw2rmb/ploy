package recipes

import (
	"fmt"
	"strconv"
	"strings"

	models "github.com/iw2rmb/ploy/api/recipes/models"
)

// PaginationInfo contains pagination metadata
type PaginationInfo struct {
	CurrentPage int  `json:"current_page"`
	PageSize    int  `json:"page_size"`
	TotalItems  int  `json:"total_items"`
	TotalPages  int  `json:"total_pages"`
	HasNext     bool `json:"has_next"`
	HasPrev     bool `json:"has_previous"`
}

// PaginatedResult represents a paginated list of recipes
type PaginatedResult struct {
	Recipes    []*models.Recipe `json:"recipes"`
	Pagination PaginationInfo   `json:"pagination"`
	Filter     RecipeFilter     `json:"filter,omitempty"`
}

// NewPaginationInfo creates pagination metadata
func NewPaginationInfo(page, pageSize, totalItems int) PaginationInfo {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	totalPages := (totalItems + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}

	return PaginationInfo{
		CurrentPage: page,
		PageSize:    pageSize,
		TotalItems:  totalItems,
		TotalPages:  totalPages,
		HasNext:     page < totalPages,
		HasPrev:     page > 1,
	}
}

// DisplayPaginationInfo shows pagination information
func DisplayPaginationInfo(info PaginationInfo) {
	startItem := (info.CurrentPage-1)*info.PageSize + 1
	endItem := info.CurrentPage * info.PageSize
	if endItem > info.TotalItems {
		endItem = info.TotalItems
	}

	if info.TotalItems == 0 {
		PrintInfo("No recipes found")
		return
	}

	fmt.Printf("\nShowing %d-%d of %d recipes (page %d of %d)\n",
		startItem, endItem, info.TotalItems, info.CurrentPage, info.TotalPages)

	// Show navigation hints
	if info.HasPrev || info.HasNext {
		var hints []string
		if info.HasPrev {
			hints = append(hints, fmt.Sprintf("Previous: --offset %d", (info.CurrentPage-2)*info.PageSize))
		}
		if info.HasNext {
			hints = append(hints, fmt.Sprintf("Next: --offset %d", info.CurrentPage*info.PageSize))
		}
		fmt.Printf("Navigation: %s\n", strings.Join(hints, " | "))
	}
}

// ApplyFilters applies filters to the recipe list
func ApplyFilters(recipes []*models.Recipe, filter RecipeFilter) []*models.Recipe {
	var filtered []*models.Recipe

	for _, recipe := range recipes {
		if !matchesFilter(recipe, filter) {
			continue
		}
		filtered = append(filtered, recipe)
	}

	return filtered
}

// matchesFilter checks if a recipe matches the given filter
func matchesFilter(recipe *models.Recipe, filter RecipeFilter) bool {
	// Language filter
	if filter.Language != "" {
		found := false
		for _, lang := range recipe.Metadata.Languages {
			if strings.EqualFold(lang, filter.Language) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Category filter
	if filter.Category != "" {
		found := false
		for _, cat := range recipe.Metadata.Categories {
			if strings.EqualFold(cat, filter.Category) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Tags filter (all tags must match)
	if len(filter.Tags) > 0 {
		for _, filterTag := range filter.Tags {
			found := false
			for _, recipeTag := range recipe.Metadata.Tags {
				if strings.EqualFold(recipeTag, filterTag) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	// Author filter
	if filter.Author != "" {
		if !strings.EqualFold(recipe.Metadata.Author, filter.Author) {
			return false
		}
	}

	// TODO: Implement MinRating filter when rating system is available

	return true
}

// ApplyPagination applies pagination to the recipe list
func ApplyPagination(recipes []*models.Recipe, offset, limit int) ([]*models.Recipe, PaginationInfo) {
	total := len(recipes)

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 20
	}

	// Calculate pagination info
	page := (offset / limit) + 1
	info := NewPaginationInfo(page, limit, total)

	// Apply pagination
	end := offset + limit
	if end > total {
		end = total
	}

	if offset >= total {
		return []*models.Recipe{}, info
	}

	return recipes[offset:end], info
}

// ParseFilterFlags parses filtering flags from command arguments
func ParseFilterFlags(args []string) (RecipeFilter, []string) {
	filter := RecipeFilter{
		Limit:  20, // Default limit
		Offset: 0,  // Default offset
	}

	// Track processed arguments
	remainingArgs := []string{}

	for i := 0; i < len(args); i++ {
		processed := false

		switch args[i] {
		case "--language", "-l":
			if i+1 < len(args) {
				filter.Language = args[i+1]
				i++
				processed = true
			}
		case "--category", "-c":
			if i+1 < len(args) {
				filter.Category = args[i+1]
				i++
				processed = true
			}
		case "--tag", "-t":
			if i+1 < len(args) {
				filter.Tags = append(filter.Tags, args[i+1])
				i++
				processed = true
			}
		case "--author", "-a":
			if i+1 < len(args) {
				filter.Author = args[i+1]
				i++
				processed = true
			}
		case "--limit":
			if i+1 < len(args) {
				if limit, err := strconv.Atoi(args[i+1]); err == nil {
					filter.Limit = limit
				}
				i++
				processed = true
			}
		case "--offset":
			if i+1 < len(args) {
				if offset, err := strconv.Atoi(args[i+1]); err == nil {
					filter.Offset = offset
				}
				i++
				processed = true
			}
		case "--sort-by":
			if i+1 < len(args) {
				filter.SortBy = args[i+1]
				i++
				processed = true
			}
		case "--sort-order":
			if i+1 < len(args) {
				filter.SortOrder = args[i+1]
				i++
				processed = true
			}
		case "--min-rating":
			if i+1 < len(args) {
				if rating, err := strconv.ParseFloat(args[i+1], 64); err == nil {
					filter.MinRating = rating
				}
				i++
				processed = true
			}
		case "--pack", "-p":
			if i+1 < len(args) {
				filter.Pack = args[i+1]
				i++
				processed = true
			}
		case "--version", "-V":
			if i+1 < len(args) {
				filter.Version = args[i+1]
				i++
				processed = true
			}
		}

		// If argument wasn't processed, add to remaining args
		if !processed {
			remainingArgs = append(remainingArgs, args[i])
		}
	}

	return filter, remainingArgs
}

// BuildAPIQuery builds query parameters for API requests
func BuildAPIQuery(filter RecipeFilter) string {
	params := []string{}

	if filter.Language != "" {
		params = append(params, "language="+filter.Language)
	}
	if filter.Category != "" {
		params = append(params, "category="+filter.Category)
	}
	for _, tag := range filter.Tags {
		params = append(params, "tag="+tag)
	}
	if filter.Author != "" {
		params = append(params, "author="+filter.Author)
	}
	if filter.Pack != "" {
		params = append(params, "pack="+filter.Pack)
	}
	if filter.Version != "" {
		params = append(params, "version="+filter.Version)
	}
	if filter.Limit > 0 {
		params = append(params, fmt.Sprintf("limit=%d", filter.Limit))
	}
	if filter.Offset > 0 {
		params = append(params, fmt.Sprintf("offset=%d", filter.Offset))
	}
	if filter.SortBy != "" {
		params = append(params, "sort_by="+filter.SortBy)
	}
	if filter.SortOrder != "" {
		params = append(params, "sort_order="+filter.SortOrder)
	}
	if filter.MinRating > 0 {
		params = append(params, fmt.Sprintf("min_rating=%.2f", filter.MinRating))
	}

	if len(params) > 0 {
		return "?" + strings.Join(params, "&")
	}
	return ""
}

// ValidateFilterValues validates filter parameter values
func ValidateFilterValues(filter RecipeFilter) error {
	// Validate sort by field
	if filter.SortBy != "" {
		validSortFields := []string{"name", "created", "updated", "author", "version", "rating"}
		valid := false
		for _, field := range validSortFields {
			if strings.EqualFold(filter.SortBy, field) {
				valid = true
				break
			}
		}
		if !valid {
			return NewCLIError(fmt.Sprintf("Invalid sort field: %s", filter.SortBy), 1).
				WithSuggestion(fmt.Sprintf("Valid fields: %s", strings.Join(validSortFields, ", ")))
		}
	}

	// Validate sort order
	if filter.SortOrder != "" {
		sortOrder := strings.ToLower(filter.SortOrder)
		if sortOrder != "asc" && sortOrder != "desc" {
			return NewCLIError(fmt.Sprintf("Invalid sort order: %s", filter.SortOrder), 1).
				WithSuggestion("Valid orders: asc, desc")
		}
	}

	// Validate limit
	if filter.Limit < 0 {
		return NewCLIError("Limit cannot be negative", 1)
	}
	if filter.Limit > 1000 {
		return NewCLIError("Limit cannot exceed 1000", 1).
			WithSuggestion("Use a smaller limit value")
	}

	// Validate offset
	if filter.Offset < 0 {
		return NewCLIError("Offset cannot be negative", 1)
	}

	// Validate rating
	if filter.MinRating < 0 || filter.MinRating > 5 {
		return NewCLIError("Rating must be between 0 and 5", 1)
	}

	return nil
}

// FormatFilterSummary creates a human-readable summary of active filters
func FormatFilterSummary(filter RecipeFilter) string {
	var parts []string

	if filter.Language != "" {
		parts = append(parts, fmt.Sprintf("language: %s", filter.Language))
	}
	if filter.Category != "" {
		parts = append(parts, fmt.Sprintf("category: %s", filter.Category))
	}
	if len(filter.Tags) > 0 {
		parts = append(parts, fmt.Sprintf("tags: %s", strings.Join(filter.Tags, ", ")))
	}
	if filter.Author != "" {
		parts = append(parts, fmt.Sprintf("author: %s", filter.Author))
	}
	if filter.MinRating > 0 {
		parts = append(parts, fmt.Sprintf("min rating: %.1f", filter.MinRating))
	}
	if filter.SortBy != "" {
		order := filter.SortOrder
		if order == "" {
			order = "asc"
		}
		parts = append(parts, fmt.Sprintf("sort: %s (%s)", filter.SortBy, order))
	}

	if len(parts) == 0 {
		return "No filters applied"
	}

	return fmt.Sprintf("Filters: %s", strings.Join(parts, ", "))
}

// DisplayAdvancedPaginatedResult shows paginated results with filter summary
func DisplayAdvancedPaginatedResult(result PaginatedResult, format string, verbose bool) error {
	// Show filter summary if filters are applied
	filterSummary := FormatFilterSummary(result.Filter)
	if filterSummary != "No filters applied" {
		fmt.Printf("🔍 %s\n\n", filterSummary)
	}

	// Format and display recipes
	if err := FormatRecipes(result.Recipes, format, verbose); err != nil {
		return err
	}

	// Display pagination info for table format
	if strings.ToLower(format) == "table" {
		DisplayPaginationInfo(result.Pagination)
	}

	return nil
}
