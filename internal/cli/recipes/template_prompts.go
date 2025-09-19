package recipes

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func collectTemplateValues(template RecipeTemplate) (map[string]string, error) {
	values := make(map[string]string)

	fmt.Printf("Please provide values for the recipe:\n\n")

	for _, prompt := range template.Prompts {
		value, err := executePrompt(prompt)
		if err != nil {
			return nil, err
		}
		values[prompt.Field] = value
	}

	return values, nil
}

func executePrompt(prompt TemplatePrompt) (string, error) {
	switch prompt.Type {
	case "select":
		return executeSelectPrompt(prompt)
	case "confirm":
		return executeConfirmPrompt(prompt)
	case "multiselect":
		return executeMultiSelectPrompt(prompt)
	default:
		return executeInputPrompt(prompt)
	}
}

func executeInputPrompt(prompt TemplatePrompt) (string, error) {
	message := prompt.Message
	if prompt.Default != "" {
		message += fmt.Sprintf(" [%s]", prompt.Default)
	}
	message += ": "

	for {
		value := promptInput(message)

		if value == "" && prompt.Default != "" {
			value = prompt.Default
		}

		if prompt.Required && value == "" {
			PrintWarning("This field is required")
			continue
		}

		return value, nil
	}
}

func executeSelectPrompt(prompt TemplatePrompt) (string, error) {
	fmt.Printf("%s:\n", prompt.Message)
	for i, option := range prompt.Options {
		marker := " "
		if option == prompt.Default {
			marker = "*"
		}
		fmt.Printf("  %s %d. %s\n", marker, i+1, option)
	}

	for {
		choice := promptInput("Select option (1-" + fmt.Sprintf("%d", len(prompt.Options)) + "): ")

		if choice == "" && prompt.Default != "" {
			return prompt.Default, nil
		}

		index, err := strconv.Atoi(choice)
		if err != nil || index < 1 || index > len(prompt.Options) {
			PrintWarning("Invalid selection")
			continue
		}

		return prompt.Options[index-1], nil
	}
}

func executeConfirmPrompt(prompt TemplatePrompt) (string, error) {
	defaultBool := strings.EqualFold(prompt.Default, "true")
	result := promptConfirm(prompt.Message, defaultBool)
	if result {
		return "true", nil
	}
	return "false", nil
}

func executeMultiSelectPrompt(prompt TemplatePrompt) (string, error) {
	fmt.Printf("%s (select multiple, comma-separated):\n", prompt.Message)
	for i, option := range prompt.Options {
		fmt.Printf("  %d. %s\n", i+1, option)
	}

	input := promptInput("Select options (e.g., 1,3,5): ")
	if input == "" {
		return prompt.Default, nil
	}

	var selected []string
	parts := strings.Split(input, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		index, err := strconv.Atoi(part)
		if err != nil || index < 1 || index > len(prompt.Options) {
			continue
		}
		selected = append(selected, prompt.Options[index-1])
	}

	return strings.Join(selected, ","), nil
}

func promptInput(message string) string {
	fmt.Print(message)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

func promptConfirm(message string, defaultValue bool) bool {
	defaultStr := "y/N"
	if defaultValue {
		defaultStr = "Y/n"
	}

	response := promptInput(fmt.Sprintf("%s (%s): ", message, defaultStr))

	if response == "" {
		return defaultValue
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}
