module github.com/example/replace

go 1.21

require (
	github.com/goliatone/go-errors v0.8.0
	github.com/replaced/module v1.0.0
)

replace github.com/replaced/module => github.com/fork/module v1.5.0