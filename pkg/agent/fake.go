package agent

// FakeSkillSpec is a placeholder skill for testing.
type FakeSkillSpec struct {
	name string
}

func NewFakeSkillSpec(name string) *FakeSkillSpec {
	return &FakeSkillSpec{name: name}
}

func (s *FakeSkillSpec) SkillName() string {
	return s.name
}
