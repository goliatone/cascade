package testdata

import (
	"os"
	"strings"
)

// MockEnvironment provides a mock environment for testing environment variable parsing.
// It allows tests to simulate different environment configurations without affecting
// the actual system environment.
type MockEnvironment struct {
	vars map[string]string
}

// NewMockEnvironment creates a new mock environment with the provided variables.
func NewMockEnvironment(vars map[string]string) *MockEnvironment {
	if vars == nil {
		vars = make(map[string]string)
	}
	return &MockEnvironment{vars: vars}
}

// Set sets an environment variable in the mock environment.
func (m *MockEnvironment) Set(key, value string) {
	m.vars[key] = value
}

// Get retrieves an environment variable from the mock environment.
func (m *MockEnvironment) Get(key string) string {
	return m.vars[key]
}

// Getenv provides the interface expected by environment parsing functions.
func (m *MockEnvironment) Getenv(key string) string {
	return m.vars[key]
}

// LookupEnv provides the interface expected by environment parsing functions
// that need to distinguish between empty values and unset variables.
func (m *MockEnvironment) LookupEnv(key string) (string, bool) {
	value, exists := m.vars[key]
	return value, exists
}

// Clear removes all variables from the mock environment.
func (m *MockEnvironment) Clear() {
	m.vars = make(map[string]string)
}

// LoadFromFile loads environment variables from a .env file into the mock environment.
// This is useful for testing with the env_config.env fixture.
func (m *MockEnvironment) LoadFromFile(filepath string) error {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			m.vars[key] = value
		}
	}

	return nil
}

// MockFileSystem provides a mock file system for testing configuration file operations.
// It simulates file existence, reading, and basic file system operations without
// requiring actual files on disk.
type MockFileSystem struct {
	files map[string][]byte
	dirs  map[string]bool
}

// NewMockFileSystem creates a new mock file system.
func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
}

// WriteFile simulates writing a file to the mock file system.
func (m *MockFileSystem) WriteFile(path string, content []byte) {
	m.files[path] = content
}

// ReadFile simulates reading a file from the mock file system.
func (m *MockFileSystem) ReadFile(path string) ([]byte, error) {
	content, exists := m.files[path]
	if !exists {
		return nil, os.ErrNotExist
	}
	return content, nil
}

// Exists checks if a file exists in the mock file system.
func (m *MockFileSystem) Exists(path string) bool {
	_, exists := m.files[path]
	return exists || m.dirs[path]
}

// MkdirAll simulates creating directories in the mock file system.
func (m *MockFileSystem) MkdirAll(path string) {
	m.dirs[path] = true
}

// IsDir checks if a path is a directory in the mock file system.
func (m *MockFileSystem) IsDir(path string) bool {
	return m.dirs[path]
}

// Clear removes all files and directories from the mock file system.
func (m *MockFileSystem) Clear() {
	m.files = make(map[string][]byte)
	m.dirs = make(map[string]bool)
}

// MockFlagParser provides a mock command-line flag parser for testing flag parsing logic.
// It allows tests to simulate different command-line argument scenarios.
type MockFlagParser struct {
	flags map[string]interface{}
	args  []string
}

// NewMockFlagParser creates a new mock flag parser.
func NewMockFlagParser() *MockFlagParser {
	return &MockFlagParser{
		flags: make(map[string]interface{}),
		args:  []string{},
	}
}

// SetString sets a string flag value.
func (m *MockFlagParser) SetString(name, value string) {
	m.flags[name] = value
}

// SetInt sets an integer flag value.
func (m *MockFlagParser) SetInt(name string, value int) {
	m.flags[name] = value
}

// SetBool sets a boolean flag value.
func (m *MockFlagParser) SetBool(name string, value bool) {
	m.flags[name] = value
}

// SetDuration sets a duration flag value.
func (m *MockFlagParser) SetDuration(name, value string) {
	m.flags[name] = value
}

// GetString retrieves a string flag value.
func (m *MockFlagParser) GetString(name string) string {
	if value, exists := m.flags[name]; exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

// GetInt retrieves an integer flag value.
func (m *MockFlagParser) GetInt(name string) int {
	if value, exists := m.flags[name]; exists {
		if i, ok := value.(int); ok {
			return i
		}
	}
	return 0
}

// GetBool retrieves a boolean flag value.
func (m *MockFlagParser) GetBool(name string) bool {
	if value, exists := m.flags[name]; exists {
		if b, ok := value.(bool); ok {
			return b
		}
	}
	return false
}

// SetArgs sets the command-line arguments for testing.
func (m *MockFlagParser) SetArgs(args []string) {
	m.args = args
}

// GetArgs retrieves the command-line arguments.
func (m *MockFlagParser) GetArgs() []string {
	return m.args
}

// Clear removes all flags and arguments from the mock parser.
func (m *MockFlagParser) Clear() {
	m.flags = make(map[string]interface{})
	m.args = []string{}
}