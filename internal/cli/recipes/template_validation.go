package recipes

func validateTemplate(template RecipeTemplate) error {
	if template.ID == "" {
		return NewCLIError("Template ID is required", 1)
	}

	if template.Name == "" {
		return NewCLIError("Template name is required", 1)
	}

	if err := template.Template.Validate(); err != nil {
		return NewCLIError("Template recipe is invalid", 1).WithCause(err)
	}

	for _, prompt := range template.Prompts {
		if prompt.Field == "" {
			return NewCLIError("Prompt field is required", 1)
		}
		if prompt.Message == "" {
			return NewCLIError("Prompt message is required", 1)
		}
	}

	return nil
}
