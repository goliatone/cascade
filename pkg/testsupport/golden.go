package testsupport

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LoadFixture reads a fixture file.
func LoadFixture(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteGolden serialises data as JSON to the golden path.
func WriteGolden(path string, data any) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0o644)
}

// LoadGolden deserialises JSON golden data into v.
func LoadGolden(path string, v any) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, v)
}

// GoldenPath resolves a golden name in testdata directory.
func GoldenPath(baseDir, name string) string {
	return filepath.Join(baseDir, name)
}
