package prompt

import "fmt"

// Fragment is the atomic, independently testable unit of a prompt.
// Each fragment has a declared Variables list, which allows the eval harness
// to auto-generate test cases and validate inputs before rendering.
type Fragment struct {
	// ID uniquely identifies the fragment within a PromptBackend.
	ID string
	// Content is the template string. Variables are referenced as {{.VarName}}.
	Content string
	// Variables declares the input variables this fragment expects.
	Variables []string
	// Tags classify the fragment (e.g. "system", "persona", "tone", "task").
	Tags []string
	// Version allows pinning to a specific revision.
	Version string
}

// Validate returns an error if any declared variable is absent from vars.
func (f *Fragment) Validate(vars map[string]any) error {
	for _, v := range f.Variables {
		if _, ok := vars[v]; !ok {
			return fmt.Errorf("fragment %q: missing variable %q", f.ID, v)
		}
	}
	return nil
}

// FragmentRef is a reference to a Fragment within a PromptTemplate.
type FragmentRef struct {
	ID        string
	Overrides map[string]any
	Condition string
}

// PromptTemplate is an ordered list of fragment references.
type PromptTemplate struct {
	Fragments []FragmentRef
	Separator string
}
