package prompt

import (
	"bytes"
	"fmt"
	"text/template"
)

// GoTemplateEngine uses Go's text/template package.
// Default engine — zero external dependencies.
type GoTemplateEngine struct{}

func (e *GoTemplateEngine) Render(tmpl string, vars map[string]any) (string, error) {
	t, err := template.New("").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("template parse: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("template execute: %w", err)
	}
	return buf.String(), nil
}

func (e *GoTemplateEngine) RenderLenient(tmpl string, vars map[string]any) (string, error) {
	t, err := template.New("").Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("template parse: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("template execute: %w", err)
	}
	return buf.String(), nil
}

func (e *GoTemplateEngine) Validate(tmpl string) error {
	_, err := template.New("").Parse(tmpl)
	return err
}
