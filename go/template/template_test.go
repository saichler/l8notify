package template

import "testing"

func TestRender_BasicSubstitution(t *testing.T) {
	result := Render("Hello {{name}}, your order {{orderId}} is {{status}}", map[string]string{
		"name":    "Alice",
		"orderId": "SO-001",
		"status":  "confirmed",
	})
	expected := "Hello Alice, your order SO-001 is confirmed"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRender_EmptyTemplate(t *testing.T) {
	result := Render("", map[string]string{"key": "value"})
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestRender_NilVars(t *testing.T) {
	tmpl := "Hello {{name}}"
	result := Render(tmpl, nil)
	if result != tmpl {
		t.Errorf("expected %q, got %q", tmpl, result)
	}
}

func TestRender_EmptyVars(t *testing.T) {
	tmpl := "Hello {{name}}"
	result := Render(tmpl, map[string]string{})
	if result != tmpl {
		t.Errorf("expected %q, got %q", tmpl, result)
	}
}

func TestRender_UnknownPlaceholdersLeftAsIs(t *testing.T) {
	result := Render("{{known}} and {{unknown}}", map[string]string{"known": "yes"})
	expected := "yes and {{unknown}}"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRender_NoPlaceholders(t *testing.T) {
	tmpl := "No placeholders here"
	result := Render(tmpl, map[string]string{"key": "value"})
	if result != tmpl {
		t.Errorf("expected %q, got %q", tmpl, result)
	}
}

func TestRender_RepeatedPlaceholder(t *testing.T) {
	result := Render("{{x}} and {{x}} again", map[string]string{"x": "val"})
	expected := "val and val again"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRender_EmptyValue(t *testing.T) {
	result := Render("before{{key}}after", map[string]string{"key": ""})
	expected := "beforeafter"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRenderWithDefault_ReplacesUnknown(t *testing.T) {
	result := RenderWithDefault("{{known}} and {{unknown}}", map[string]string{"known": "yes"}, "N/A")
	expected := "yes and N/A"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRenderWithDefault_EmptyTemplate(t *testing.T) {
	result := RenderWithDefault("", map[string]string{"key": "value"}, "N/A")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestRenderWithDefault_AllKnown(t *testing.T) {
	result := RenderWithDefault("{{a}} {{b}}", map[string]string{"a": "1", "b": "2"}, "N/A")
	expected := "1 2"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRenderWithDefault_MultipleUnknown(t *testing.T) {
	result := RenderWithDefault("{{a}} {{b}} {{c}}", map[string]string{"b": "yes"}, "-")
	expected := "- yes -"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}
