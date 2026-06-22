package prompt_test

import (
	"context"
	"testing"

	"github.com/stlimtat/bunshin-go/pkg/prompt"
)

const testTenant = "test-tenant"

func newBackendWith(frags ...*prompt.Fragment) *prompt.MemoryBackend {
	b := prompt.NewMemoryBackend()
	for _, f := range frags {
		_ = b.Put(context.Background(), testTenant, f)
	}
	return b
}

// ---- Fragment.Validate ----

func TestFragment_Validate_OK(t *testing.T) {
	f := &prompt.Fragment{Slug: "f1", Variables: []string{"name", "domain"}}
	if err := f.Validate(map[string]any{"name": "Alice", "domain": "law"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFragment_Validate_MissingVar(t *testing.T) {
	f := &prompt.Fragment{Slug: "f1", Variables: []string{"name"}}
	if err := f.Validate(map[string]any{}); err == nil {
		t.Fatal("expected error for missing variable")
	}
}

func TestFragment_Validate_NoVars(t *testing.T) {
	f := &prompt.Fragment{Slug: "static"}
	if err := f.Validate(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---- GoTemplateEngine ----

func TestGoTemplateEngine_Render(t *testing.T) {
	e := &prompt.GoTemplateEngine{}
	out, err := e.Render("Hello {{.name}}!", map[string]any{"name": "World"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "Hello World!" {
		t.Fatalf("want 'Hello World!', got %q", out)
	}
}

func TestGoTemplateEngine_Render_MissingKey(t *testing.T) {
	e := &prompt.GoTemplateEngine{}
	_, err := e.Render("{{.missing}}", map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing key with missingkey=error")
	}
}

func TestGoTemplateEngine_Validate_InvalidSyntax(t *testing.T) {
	e := &prompt.GoTemplateEngine{}
	if err := e.Validate("{{.unclosed"); err == nil {
		t.Fatal("expected syntax error")
	}
}

// ---- MemoryBackend ----

func TestMemoryBackend_GetMissing(t *testing.T) {
	b := prompt.NewMemoryBackend()
	_, err := b.Get(context.Background(), testTenant, "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMemoryBackend_PutAndGet(t *testing.T) {
	b := prompt.NewMemoryBackend()
	f := &prompt.Fragment{Slug: "greet", Content: "Hello {{.name}}"}
	_ = b.Put(context.Background(), testTenant, f)
	got, err := b.Get(context.Background(), testTenant, "greet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Slug != "greet" {
		t.Fatalf("want greet, got %q", got.Slug)
	}
	if got.ID == "" {
		t.Fatal("UUID must be generated on Put")
	}
}

func TestMemoryBackend_PutGeneratesUUID(t *testing.T) {
	b := prompt.NewMemoryBackend()
	f := &prompt.Fragment{Slug: "s1"}
	_ = b.Put(context.Background(), testTenant, f)
	if f.ID == "" {
		t.Fatal("Put must generate UUID when ID is empty")
	}
}

func TestMemoryBackend_GetByID(t *testing.T) {
	b := prompt.NewMemoryBackend()
	f := &prompt.Fragment{Slug: "s2", Content: "content"}
	_ = b.Put(context.Background(), testTenant, f)
	got, err := b.GetByID(context.Background(), testTenant, f.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Slug != "s2" {
		t.Errorf("want slug=s2, got %q", got.Slug)
	}
}

func TestMemoryBackend_List_NoFilter(t *testing.T) {
	b := newBackendWith(
		&prompt.Fragment{Slug: "a", Tags: []string{"system"}},
		&prompt.Fragment{Slug: "b", Tags: []string{"user"}},
	)
	frags, _ := b.List(context.Background(), testTenant)
	if len(frags) != 2 {
		t.Fatalf("want 2, got %d", len(frags))
	}
}

func TestMemoryBackend_List_TagFilter(t *testing.T) {
	b := newBackendWith(
		&prompt.Fragment{Slug: "a", Tags: []string{"system", "persona"}},
		&prompt.Fragment{Slug: "b", Tags: []string{"user"}},
		&prompt.Fragment{Slug: "c", Tags: []string{"system"}},
	)
	frags, _ := b.List(context.Background(), testTenant, "system")
	if len(frags) != 2 {
		t.Fatalf("want 2 system frags, got %d", len(frags))
	}
}

func TestMemoryBackend_GetVersion(t *testing.T) {
	b := prompt.NewMemoryBackend()
	_ = b.Put(context.Background(), testTenant, &prompt.Fragment{Slug: "f", Version: "v1", Content: "old"})
	_ = b.Put(context.Background(), testTenant, &prompt.Fragment{Slug: "f", Version: "v2", Content: "new"})

	v1, err := b.GetVersion(context.Background(), testTenant, "f", "v1")
	if err != nil || v1.Content != "old" {
		t.Fatalf("want old, got %q %v", v1.Content, err)
	}
	v2, err := b.GetVersion(context.Background(), testTenant, "f", "v2")
	if err != nil || v2.Content != "new" {
		t.Fatalf("want new, got %q %v", v2.Content, err)
	}
}

func TestMemoryBackend_Rename(t *testing.T) {
	b := prompt.NewMemoryBackend()
	f := &prompt.Fragment{Slug: "old-slug", Content: "c"}
	_ = b.Put(context.Background(), testTenant, f)
	id := f.ID

	if err := b.Rename(context.Background(), testTenant, id, "new-slug"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	// Old slug gone.
	if _, err := b.Get(context.Background(), testTenant, "old-slug"); err == nil {
		t.Error("old slug should be gone after rename")
	}
	// New slug works.
	got, err := b.Get(context.Background(), testTenant, "new-slug")
	if err != nil || got.Slug != "new-slug" {
		t.Fatalf("new slug not found: %v", err)
	}
}

func TestMemoryBackend_TenantIsolation(t *testing.T) {
	b := prompt.NewMemoryBackend()
	_ = b.Put(context.Background(), "tenant-a", &prompt.Fragment{Slug: "shared", Content: "a"})
	_, err := b.Get(context.Background(), "tenant-b", "shared")
	if err == nil {
		t.Error("tenant-b must not see tenant-a fragments")
	}
}

func TestMemoryBackend_Watch_ReceivesUpdate(t *testing.T) {
	b := prompt.NewMemoryBackend()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := b.Watch(ctx, testTenant, "myf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = b.Put(context.Background(), testTenant, &prompt.Fragment{Slug: "myf", Content: "v1"})

	f := <-ch
	if f.Content != "v1" {
		t.Fatalf("want v1, got %q", f.Content)
	}
}

// ---- PromptComposer ----

func TestPromptComposer_Render_Simple(t *testing.T) {
	b := newBackendWith(&prompt.Fragment{
		Slug:      "greet",
		Content:   "Hello {{.name}}!",
		Variables: []string{"name"},
	})
	c := prompt.NewPromptComposer(b, testTenant)
	tmpl := prompt.PromptTemplate{
		Fragments: []prompt.FragmentRef{{Slug: "greet"}},
	}
	out, err := c.Render(context.Background(), tmpl, map[string]any{"name": "Alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "Hello Alice!" {
		t.Fatalf("want 'Hello Alice!', got %q", out)
	}
}

func TestPromptComposer_Render_MultiFragment(t *testing.T) {
	b := newBackendWith(
		&prompt.Fragment{Slug: "a", Content: "Part A"},
		&prompt.Fragment{Slug: "b", Content: "Part B"},
	)
	c := prompt.NewPromptComposer(b, testTenant)
	tmpl := prompt.PromptTemplate{
		Fragments: []prompt.FragmentRef{{Slug: "a"}, {Slug: "b"}},
		Separator: " | ",
	}
	out, _ := c.Render(context.Background(), tmpl, nil)
	if out != "Part A | Part B" {
		t.Fatalf("want 'Part A | Part B', got %q", out)
	}
}

func TestPromptComposer_Render_Override(t *testing.T) {
	b := newBackendWith(&prompt.Fragment{
		Slug:      "persona",
		Content:   "You are a {{.role}} expert.",
		Variables: []string{"role"},
	})
	c := prompt.NewPromptComposer(b, testTenant)
	tmpl := prompt.PromptTemplate{
		Fragments: []prompt.FragmentRef{
			{Slug: "persona", Overrides: map[string]any{"role": "legal"}},
		},
	}
	out, _ := c.Render(context.Background(), tmpl, map[string]any{"role": "generic"})
	if out != "You are a legal expert." {
		t.Fatalf("override not applied, got %q", out)
	}
}

func TestPromptComposer_Render_MissingFragment(t *testing.T) {
	b := prompt.NewMemoryBackend()
	c := prompt.NewPromptComposer(b, testTenant)
	tmpl := prompt.PromptTemplate{
		Fragments: []prompt.FragmentRef{{Slug: "nonexistent"}},
	}
	_, err := c.Render(context.Background(), tmpl, nil)
	if err == nil {
		t.Fatal("expected error for missing fragment")
	}
}

func TestPromptComposer_Render_MissingVariable(t *testing.T) {
	b := newBackendWith(&prompt.Fragment{
		Slug:      "needs-var",
		Content:   "{{.required}}",
		Variables: []string{"required"},
	})
	c := prompt.NewPromptComposer(b, testTenant)
	tmpl := prompt.PromptTemplate{
		Fragments: []prompt.FragmentRef{{Slug: "needs-var"}},
	}
	_, err := c.Render(context.Background(), tmpl, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing variable")
	}
}
