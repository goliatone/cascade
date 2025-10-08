package main

import "embed"

// Embedded assets for dependency-registration scaffolding.
//
//go:embed ../scripts/detect-new-goliatone-deps.sh
var dependencyRegistrationScript []byte

//go:embed templates/dependency_registration/config.yml
var dependencyRegistrationConfig []byte

//go:embed templates/dependency_registration/README.md
var dependencyRegistrationReadme []byte

//go:embed ../.github/workflows/dependency-registration.yml.tpl
var dependencyRegistrationWorkflow []byte
