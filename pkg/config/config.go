package config

// Builder orchestrates config assembly from various sources.
type Builder interface {
	FromEnv() Builder
	FromFlags(args []string) Builder
	FromFile(path string) Builder
	Build() (*Config, error)
}

// NewBuilder returns a stub builder that currently returns NotImplemented errors.
func NewBuilder() Builder {
	return &stubBuilder{}
}

type stubBuilder struct{}

func (b *stubBuilder) FromEnv() Builder {
	return b
}

func (b *stubBuilder) FromFlags(args []string) Builder {
	_ = args
	return b
}

func (b *stubBuilder) FromFile(path string) Builder {
	_ = path
	return b
}

func (b *stubBuilder) Build() (*Config, error) {
	return nil, newNotImplemented("Builder.Build")
}
