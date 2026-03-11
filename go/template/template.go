package template

import "strings"

// Render replaces all {{key}} placeholders in tmpl with values from vars.
// Unknown placeholders are left as-is. Returns the rendered string.
func Render(tmpl string, vars map[string]string) string {
	if tmpl == "" || len(vars) == 0 {
		return tmpl
	}
	result := tmpl
	for key, value := range vars {
		placeholder := "{{" + key + "}}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// RenderWithDefault replaces placeholders; unknown keys get defaultValue.
func RenderWithDefault(tmpl string, vars map[string]string, defaultValue string) string {
	if tmpl == "" {
		return tmpl
	}

	result := Render(tmpl, vars)

	// Replace any remaining {{...}} placeholders with the default value
	for {
		start := strings.Index(result, "{{")
		if start < 0 {
			break
		}
		end := strings.Index(result[start:], "}}")
		if end < 0 {
			break
		}
		end += start + 2
		result = result[:start] + defaultValue + result[end:]
	}
	return result
}
