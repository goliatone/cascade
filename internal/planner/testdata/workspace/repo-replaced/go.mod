module github.com/goliatone/repo-replaced

go 1.21

require (
	github.com/goliatone/go-errors v0.8.0
	github.com/other/dependency v1.2.3
)

replace github.com/goliatone/go-errors => ../local/go-errors