package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPaths(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single path",
			input:    "test/*.xml",
			expected: []string{"test/*.xml"},
		},
		{
			name:     "multiple paths",
			input:    "test1/*.xml,test2/*.xml,test3/*.xml",
			expected: []string{"test1/*.xml", "test2/*.xml", "test3/*.xml"},
		},
		{
			name:     "paths with spaces",
			input:    "test1/*.xml, test2/*.xml , test3/*.xml",
			expected: []string{"test1/*.xml", "test2/*.xml", "test3/*.xml"},
		},
		{
			name:     "paths with empty segments",
			input:    "test1/*.xml,,test3/*.xml",
			expected: []string{"test1/*.xml", "test3/*.xml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPaths(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUniqueItems(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string(nil),
		},
		{
			name:     "no duplicates",
			input:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "with duplicates",
			input:    []string{"a", "b", "a", "c", "b"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "all duplicates",
			input:    []string{"a", "a", "a"},
			expected: []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := uniqueItems(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandTilde(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty path",
			input:       "",
			expectError: false,
		},
		{
			name:        "path without tilde",
			input:       "/usr/local/bin",
			expectError: false,
		},
		{
			name:        "path with tilde",
			input:       "~/documents",
			expectError: false,
		},
		{
			name:        "path with tilde and slash",
			input:       "~/documents/file.txt",
			expectError: false,
		},
		{
			name:        "user-specific home dir",
			input:       "~user/documents",
			expectError: true,
			errorMsg:    "cannot expand user-specific home dir",
		},
		{
			name:        "just tilde",
			input:       "~",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandTilde(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				if tt.input == "" {
					assert.Equal(t, "", result)
				} else if !strings.HasPrefix(tt.input, "~") {
					assert.Equal(t, tt.input, result)
				} else {
					// For paths starting with ~, verify they're expanded
					assert.NotEqual(t, tt.input, result)
					assert.NotContains(t, result, "~")
				}
			}
		})
	}
}

func TestIsURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "http url",
			input:    "http://example.com",
			expected: true,
		},
		{
			name:     "https url",
			input:    "https://example.com",
			expected: true,
		},
		{
			name:     "local file path",
			input:    "/path/to/file.yaml",
			expected: false,
		},
		{
			name:     "relative path",
			input:    "./file.yaml",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadYAML(t *testing.T) {
	t.Run("load from local file", func(t *testing.T) {
		// Create a temporary YAML file
		tempFile, err := os.CreateTemp("", "test-*.yaml")
		require.NoError(t, err)
		defer os.Remove(tempFile.Name())

		yamlContent := `
quarantine_tests:
  - name: TestFoo
    classname: com.example.TestFoo
    start_date: "2023-01-01"
    end_date: "2023-12-31"
`
		_, err = tempFile.WriteString(yamlContent)
		require.NoError(t, err)
		tempFile.Close()

		result, err := LoadYAML(tempFile.Name())
		require.NoError(t, err)
		assert.Contains(t, result, "quarantine_tests")
	})

	t.Run("load from URL", func(t *testing.T) {
		// Create a test HTTP server
		yamlContent := `
quarantine_tests:
  - name: TestBar
    classname: com.example.TestBar
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/x-yaml")
			io.WriteString(w, yamlContent)
		}))
		defer server.Close()

		result, err := LoadYAML(server.URL)
		require.NoError(t, err)
		assert.Contains(t, result, "quarantine_tests")
	})

	t.Run("invalid YAML", func(t *testing.T) {
		tempFile, err := os.CreateTemp("", "test-*.yaml")
		require.NoError(t, err)
		defer os.Remove(tempFile.Name())

		invalidYAML := `
invalid: yaml: content:
  - [unclosed
`
		_, err = tempFile.WriteString(invalidYAML)
		require.NoError(t, err)
		tempFile.Close()

		_, err = LoadYAML(tempFile.Name())
		assert.Error(t, err)
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := LoadYAML("/nonexistent/file.yaml")
		assert.Error(t, err)
	})
}

func TestParseTests(t *testing.T) {
	// Create a temporary test XML file
	tempDir, err := os.MkdirTemp("", "test-reports-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="TestSuite" tests="2" failures="1" errors="0" time="0.5">
	<testcase name="TestPassed" classname="com.example.TestPassed" time="0.2">
	</testcase>
	<testcase name="TestFailed" classname="com.example.TestFailed" time="0.3">
		<failure message="Test failed">Test failure details</failure>
	</testcase>
</testsuite>`

	testFile := filepath.Join(tempDir, "test-results.xml")
	err = os.WriteFile(testFile, []byte(xmlContent), 0644)
	require.NoError(t, err)

	logger := logrus.New()
	logger.SetOutput(io.Discard) // Suppress log output during tests

	t.Run("successful parsing with failures", func(t *testing.T) {
		paths := []string{testFile}
		stats, err := ParseTests(paths, logger)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed tests and errors found")
		assert.Equal(t, 2, stats.TestCount)
		assert.Equal(t, 1, stats.PassCount)
		assert.Equal(t, 1, stats.FailCount)
		assert.Equal(t, 0, stats.SkippedCount)
		assert.Equal(t, 0, stats.ErrorCount)
	})

	t.Run("no matching files", func(t *testing.T) {
		paths := []string{"/nonexistent/*.xml"}
		stats, err := ParseTests(paths, logger)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not find any files matching the provided report path")
		assert.Equal(t, 0, stats.TestCount)
	})
}

func TestParseTestsWithQuarantine(t *testing.T) {
	// Create a temporary test XML file
	tempDir, err := os.MkdirTemp("", "test-reports-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="TestSuite" tests="3" failures="2" errors="0" time="0.8">
	<testcase name="TestPassed" classname="com.example.TestPassed" time="0.2">
	</testcase>
	<testcase name="TestQuarantined" classname="com.example.TestQuarantined" time="0.3">
		<failure message="Test failed">Test failure details</failure>
	</testcase>
	<testcase name="TestFailed" classname="com.example.TestFailed" time="0.3">
		<failure message="Test failed">Test failure details</failure>
	</testcase>
</testsuite>`

	testFile := filepath.Join(tempDir, "test-results.xml")
	err = os.WriteFile(testFile, []byte(xmlContent), 0644)
	require.NoError(t, err)

	logger := logrus.New()
	logger.SetOutput(io.Discard) // Suppress log output during tests

	t.Run("with quarantine list", func(t *testing.T) {
		currentTime := time.Now()
		startDate := currentTime.AddDate(0, 0, -10).Format("2006-01-02")
		endDate := currentTime.AddDate(0, 0, 10).Format("2006-01-02")
		
		quarantineList := map[string]interface{}{
			"quarantine_tests": []interface{}{
				map[interface{}]interface{}{
					"name":       "TestQuarantined",
					"classname":  "com.example.TestQuarantined",
					"start_date": startDate,
					"end_date":   endDate,
				},
			},
		}

		paths := []string{testFile}
		stats, err := ParseTestsWithQuarantine(paths, quarantineList, logger)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Non-quarantined failures: 1")
		assert.Equal(t, 3, stats.TestCount)
		assert.Equal(t, 1, stats.PassCount)
		assert.Equal(t, 2, stats.FailCount)
		assert.Len(t, stats.NonQuarantinedFailuresList, 1)
		assert.Contains(t, stats.NonQuarantinedFailuresList, "com.example.TestFailed.TestFailed")
		assert.Len(t, stats.QuarantinedFailuresList, 1)
		assert.Contains(t, stats.QuarantinedFailuresList, "com.example.TestQuarantined.TestQuarantined")
		assert.Len(t, stats.ExpiredTestsList, 0) // Should not be expired
	})

	t.Run("with expired quarantine", func(t *testing.T) {
		quarantineList := map[string]interface{}{
			"quarantine_tests": []interface{}{
				map[interface{}]interface{}{
					"name":       "TestQuarantined",
					"classname":  "com.example.TestQuarantined",
					"start_date": "2020-01-01",
					"end_date":   "2020-12-31", // Expired
				},
			},
		}

		paths := []string{testFile}
		stats, err := ParseTestsWithQuarantine(paths, quarantineList, logger)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Expired tests: 1")
		assert.Len(t, stats.ExpiredTestsList, 1)
		assert.Contains(t, stats.ExpiredTestsList, "com.example.TestQuarantined.TestQuarantined")
	})

	t.Run("with quarantine ending today", func(t *testing.T) {
		currentTime := time.Now()
		startDate := currentTime.AddDate(0, 0, -10).Format("2006-01-02")
		endDate := currentTime.Format("2006-01-02") // Ends today
		
		quarantineList := map[string]interface{}{
			"quarantine_tests": []interface{}{
				map[interface{}]interface{}{
					"name":       "TestQuarantined",
					"classname":  "com.example.TestQuarantined",
					"start_date": startDate,
					"end_date":   endDate,
				},
			},
		}

		paths := []string{testFile}
		stats, err := ParseTestsWithQuarantine(paths, quarantineList, logger)
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Expired tests: 1")
		assert.Len(t, stats.ExpiredTestsList, 1)
		assert.Contains(t, stats.ExpiredTestsList, "com.example.TestQuarantined.TestQuarantined")
	})
}

func TestQuarantineEndDateLogic(t *testing.T) {
	// Create a temporary test XML file with a failing test
	tempDir, err := os.MkdirTemp("", "test-reports-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="TestSuite" tests="1" failures="1" errors="0" time="0.3">
	<testcase name="TestQuarantined" classname="com.example.TestQuarantined" time="0.3">
		<failure message="Test failed">Test failure details</failure>
	</testcase>
</testsuite>`

	testFile := filepath.Join(tempDir, "test-results.xml")
	err = os.WriteFile(testFile, []byte(xmlContent), 0644)
	require.NoError(t, err)

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	currentTime := time.Now()
	paths := []string{testFile}

	t.Run("end_date is yesterday - should be expired", func(t *testing.T) {
		startDate := currentTime.AddDate(0, 0, -10).Format("2006-01-02")
		endDate := currentTime.AddDate(0, 0, -1).Format("2006-01-02") // Yesterday
		
		quarantineList := map[string]interface{}{
			"quarantine_tests": []interface{}{
				map[interface{}]interface{}{
					"name":       "TestQuarantined",
					"classname":  "com.example.TestQuarantined",
					"start_date": startDate,
					"end_date":   endDate,
				},
			},
		}

		stats, err := ParseTestsWithQuarantine(paths, quarantineList, logger)
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Expired tests: 1")
		assert.Len(t, stats.ExpiredTestsList, 1)
		assert.Contains(t, stats.ExpiredTestsList, "com.example.TestQuarantined.TestQuarantined")
		assert.Len(t, stats.QuarantinedFailuresList, 0) // Should not be quarantined since expired
	})

	t.Run("end_date is today - should be expired", func(t *testing.T) {
		startDate := currentTime.AddDate(0, 0, -10).Format("2006-01-02")
		endDate := currentTime.Format("2006-01-02") // Today
		
		quarantineList := map[string]interface{}{
			"quarantine_tests": []interface{}{
				map[interface{}]interface{}{
					"name":       "TestQuarantined",
					"classname":  "com.example.TestQuarantined",
					"start_date": startDate,
					"end_date":   endDate,
				},
			},
		}

		stats, err := ParseTestsWithQuarantine(paths, quarantineList, logger)
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Expired tests: 1")
		assert.Len(t, stats.ExpiredTestsList, 1)
		assert.Contains(t, stats.ExpiredTestsList, "com.example.TestQuarantined.TestQuarantined")
		assert.Len(t, stats.QuarantinedFailuresList, 0) // Should not be quarantined since expired
	})

	t.Run("end_date is tomorrow - should be active", func(t *testing.T) {
		startDate := currentTime.AddDate(0, 0, -10).Format("2006-01-02")
		endDate := currentTime.AddDate(0, 0, 1).Format("2006-01-02") // Tomorrow
		
		quarantineList := map[string]interface{}{
			"quarantine_tests": []interface{}{
				map[interface{}]interface{}{
					"name":       "TestQuarantined",
					"classname":  "com.example.TestQuarantined",
					"start_date": startDate,
					"end_date":   endDate,
				},
			},
		}

		stats, err := ParseTestsWithQuarantine(paths, quarantineList, logger)
		
		// Should not error because the test is properly quarantined
		assert.NoError(t, err)
		assert.Len(t, stats.ExpiredTestsList, 0)
		assert.Len(t, stats.QuarantinedFailuresList, 1) // Should be quarantined
		assert.Contains(t, stats.QuarantinedFailuresList, "com.example.TestQuarantined.TestQuarantined")
	})
}

func TestIsQuarantined(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	quarantineList := map[string]interface{}{
		"quarantine_tests": []interface{}{
			map[interface{}]interface{}{
				"name":      "TestFoo",
				"classname": "com.example.TestFoo",
			},
			map[interface{}]interface{}{
				"name":      "TestBar",
				"classname": "com.example.TestBar",
			},
		},
	}

	tests := []struct {
		name           string
		testIdentifier string
		expected       bool
	}{
		{
			name:           "quarantined test",
			testIdentifier: "com.example.TestFoo.TestFoo",
			expected:       true,
		},
		{
			name:           "non-quarantined test",
			testIdentifier: "com.example.TestBaz.TestBaz",
			expected:       false,
		},
		{
			name:           "another quarantined test",
			testIdentifier: "com.example.TestBar.TestBar",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isQuarantined(tt.testIdentifier, quarantineList, logger)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsExpired(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	currentTime := time.Now()
	pastDate := currentTime.AddDate(0, 0, -10).Format("2006-01-02")
	futureDate := currentTime.AddDate(0, 0, 10).Format("2006-01-02")
	expiredEndDate := currentTime.AddDate(0, 0, -1).Format("2006-01-02")

	quarantineList := map[string]interface{}{
		"quarantine_tests": []interface{}{
			map[interface{}]interface{}{
				"name":       "TestActive",
				"classname":  "com.example.TestActive",
				"start_date": pastDate,
				"end_date":   futureDate,
			},
			map[interface{}]interface{}{
				"name":       "TestExpired",
				"classname":  "com.example.TestExpired",
				"start_date": pastDate,
				"end_date":   expiredEndDate,
			},
			map[interface{}]interface{}{
				"name":      "TestNoDates",
				"classname": "com.example.TestNoDates",
			},
		},
	}

	tests := []struct {
		name           string
		testIdentifier string
		expected       bool
	}{
		{
			name:           "active quarantined test",
			testIdentifier: "com.example.TestActive.TestActive",
			expected:       false,
		},
		{
			name:           "expired quarantined test",
			testIdentifier: "com.example.TestExpired.TestExpired",
			expected:       true,
		},
		{
			name:           "test with no dates",
			testIdentifier: "com.example.TestNoDates.TestNoDates",
			expected:       false,
		},
		{
			name:           "non-quarantined test",
			testIdentifier: "com.example.TestOther.TestOther",
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isExpired(tt.testIdentifier, quarantineList, logger)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchTestIdentifier(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	tests := []struct {
		name          string
		testMap       map[interface{}]interface{}
		identifier    string
		expectedMatch string
		expectedFound bool
	}{
		{
			name: "matching test",
			testMap: map[interface{}]interface{}{
				"name":      "TestFoo",
				"classname": "com.example.TestFoo",
			},
			identifier:    "com.example.TestFoo.TestFoo",
			expectedMatch: "com.example.TestFoo.TestFoo",
			expectedFound: true,
		},
		{
			name: "non-matching test",
			testMap: map[interface{}]interface{}{
				"name":      "TestFoo",
				"classname": "com.example.TestFoo",
			},
			identifier:    "com.example.TestBar.TestBar",
			expectedMatch: "",
			expectedFound: false,
		},
		{
			name: "missing classname",
			testMap: map[interface{}]interface{}{
				"name": "TestFoo",
			},
			identifier:    "com.example.TestFoo.TestFoo",
			expectedMatch: "",
			expectedFound: false,
		},
		{
			name: "missing name",
			testMap: map[interface{}]interface{}{
				"classname": "com.example.TestFoo",
			},
			identifier:    "com.example.TestFoo.TestFoo",
			expectedMatch: "",
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, found := matchTestIdentifier(tt.testMap, tt.identifier, logger)
			assert.Equal(t, tt.expectedMatch, match)
			assert.Equal(t, tt.expectedFound, found)
		})
	}
}
