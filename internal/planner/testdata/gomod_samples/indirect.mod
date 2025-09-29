module github.com/example/indirect

go 1.21

require (
	github.com/goliatone/go-errors v0.8.0
	github.com/direct/dependency v1.0.0
)

require github.com/indirect/dependency v0.5.0 // indirect