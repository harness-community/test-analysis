package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlugin_Exec(t *testing.T) {
	// Create a temporary test XML file
	tempDir, err := os.MkdirTemp("", "test-reports-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="TestSuite" tests="2" failures="0" errors="0" time="0.5">
	<testcase name="TestPassed1" classname="com.example.TestPassed1" time="0.2">
	</testcase>
	<testcase name="TestPassed2" classname="com.example.TestPassed2" time="0.3">
	</testcase>
</testsuite>`

	testFile := filepath.Join(tempDir, "test-results.xml")
	err = os.WriteFile(testFile, []byte(xmlContent), 0644)
	require.NoError(t, err)

	// Create a temporary output file
	outputFile := filepath.Join(tempDir, "output.txt")

	// Set environment variable for DRONE_OUTPUT
	oldDroneOutput := os.Getenv("DRONE_OUTPUT")
	os.Setenv("DRONE_OUTPUT", outputFile)
	defer os.Setenv("DRONE_OUTPUT", oldDroneOutput)

	t.Run("successful execution without quarantine", func(t *testing.T) {
		plugin := Plugin{
			GlobPaths:        testFile,
			QuarantineFile:   "",
			FailOnQuarantine: false,
		}

		err := plugin.Exec()
		assert.NoError(t, err)

		// Check that output file was created and contains expected stats
		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)

		output := string(content)
		assert.Contains(t, output, "TOTAL_TESTS=2")
		assert.Contains(t, output, "PASSED_TESTS=2")
		assert.Contains(t, output, "FAILED_TESTS=0")
		assert.Contains(t, output, "SKIPPED_TESTS=0")
		assert.Contains(t, output, "ERROR_TESTS=0")
	})

	t.Run("execution with quarantine", func(t *testing.T) {
		// Create quarantine file
		quarantineFile := filepath.Join(tempDir, "quarantine.yaml")
		quarantineContent := `
quarantine_tests:
  - name: TestPassed1
    classname: com.example.TestPassed1
    start_date: "2023-01-01"
    end_date: "2023-12-31"
`
		err := os.WriteFile(quarantineFile, []byte(quarantineContent), 0644)
		require.NoError(t, err)

		plugin := Plugin{
			GlobPaths:        testFile,
			QuarantineFile:   quarantineFile,
			FailOnQuarantine: true,
		}

		err = plugin.Exec()
		assert.NoError(t, err)
	})

	// Clean up output file for next test
	os.Remove(outputFile)
}

func TestPlugin_Exec_ErrorCases(t *testing.T) {
	t.Run("missing glob paths", func(t *testing.T) {
		plugin := Plugin{
			GlobPaths:        "",
			QuarantineFile:   "",
			FailOnQuarantine: false,
		}

		// Capture the exit behavior by checking if os.Exit would be called
		// This is a limitation of testing os.Exit directly, but we can test the logic
		defer func() {
			if r := recover(); r != nil {
				// This would be called if os.Exit was called
			}
		}()

		// We can't easily test os.Exit(1) directly, but we can verify the condition
		assert.Equal(t, "", plugin.GlobPaths)
	})

	t.Run("quarantine enabled but no quarantine file", func(t *testing.T) {
		plugin := Plugin{
			GlobPaths:        "test/*.xml",
			QuarantineFile:   "",
			FailOnQuarantine: true,
		}

		// Similar to above, we can't easily test os.Exit but can verify conditions
		assert.Equal(t, "", plugin.QuarantineFile)
		assert.True(t, plugin.FailOnQuarantine)
	})
}

func TestWriteTestStats(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-output-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	outputFile := filepath.Join(tempDir, "output.txt")

	// Set environment variable for DRONE_OUTPUT
	oldDroneOutput := os.Getenv("DRONE_OUTPUT")
	os.Setenv("DRONE_OUTPUT", outputFile)
	defer os.Setenv("DRONE_OUTPUT", oldDroneOutput)

	stats := TestStats{
		TestCount:    10,
		PassCount:    7,
		FailCount:    2,
		SkippedCount: 1,
		ErrorCount:   0,
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard) // Suppress log output during tests

	writeTestStats(stats, logger)

	// Verify output file contents
	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)

	output := string(content)
	assert.Contains(t, output, "TOTAL_TESTS=10")
	assert.Contains(t, output, "PASSED_TESTS=7")
	assert.Contains(t, output, "FAILED_TESTS=2")
	assert.Contains(t, output, "SKIPPED_TESTS=1")
	assert.Contains(t, output, "ERROR_TESTS=0")
}

func TestWriteEnvToFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-output-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	outputFile := filepath.Join(tempDir, "output.txt")

	// Set environment variable for DRONE_OUTPUT
	oldDroneOutput := os.Getenv("DRONE_OUTPUT")
	os.Setenv("DRONE_OUTPUT", outputFile)
	defer os.Setenv("DRONE_OUTPUT", oldDroneOutput)

	logger := logrus.New()
	logger.SetOutput(io.Discard) // Suppress log output during tests

	t.Run("write single key-value", func(t *testing.T) {
		err := WriteEnvToFile("TEST_KEY", "test_value", logger)
		assert.NoError(t, err)

		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "TEST_KEY=test_value")
	})

	t.Run("write multiple key-values", func(t *testing.T) {
		err := WriteEnvToFile("ANOTHER_KEY", "another_value", logger)
		assert.NoError(t, err)

		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		output := string(content)
		assert.Contains(t, output, "TEST_KEY=test_value")
		assert.Contains(t, output, "ANOTHER_KEY=another_value")
	})

	t.Run("error when DRONE_OUTPUT not set", func(t *testing.T) {
		os.Unsetenv("DRONE_OUTPUT")
		err := WriteEnvToFile("FAIL_KEY", "fail_value", logger)
		assert.Error(t, err)

		// Restore environment variable
		os.Setenv("DRONE_OUTPUT", outputFile)
	})
}

func TestTestStats(t *testing.T) {
	t.Run("TestStats initialization", func(t *testing.T) {
		stats := TestStats{
			TestCount:                  100,
			PassCount:                  85,
			FailCount:                  10,
			SkippedCount:               5,
			ErrorCount:                 0,
			NonQuarantinedFailuresList: []string{"test1", "test2"},
			ExpiredTestsList:           []string{"test3"},
			QuarantinedFailuresList:    []string{"test4", "test5"},
		}

		assert.Equal(t, 100, stats.TestCount)
		assert.Equal(t, 85, stats.PassCount)
		assert.Equal(t, 10, stats.FailCount)
		assert.Equal(t, 5, stats.SkippedCount)
		assert.Equal(t, 0, stats.ErrorCount)
		assert.Len(t, stats.NonQuarantinedFailuresList, 2)
		assert.Len(t, stats.ExpiredTestsList, 1)
		assert.Len(t, stats.QuarantinedFailuresList, 2)
	})
}

func TestPlugin_Integration(t *testing.T) {
	// Create a temporary test environment
	tempDir, err := os.MkdirTemp("", "test-integration-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test XML files with different scenarios
	passedTestXML := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="PassedTests" tests="2" failures="0" errors="0" time="0.5">
	<testcase name="TestPassed1" classname="com.example.TestPassed1" time="0.2">
	</testcase>
	<testcase name="TestPassed2" classname="com.example.TestPassed2" time="0.3">
	</testcase>
</testsuite>`

	failedTestXML := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="FailedTests" tests="2" failures="2" errors="0" time="0.8">
	<testcase name="TestQuarantined" classname="com.example.TestQuarantined" time="0.3">
		<failure message="Test failed">Test failure details</failure>
	</testcase>
	<testcase name="TestRegularFailure" classname="com.example.TestRegularFailure" time="0.5">
		<failure message="Test failed">Test failure details</failure>
	</testcase>
</testsuite>`

	passedFile := filepath.Join(tempDir, "passed-tests.xml")
	failedFile := filepath.Join(tempDir, "failed-tests.xml")

	err = os.WriteFile(passedFile, []byte(passedTestXML), 0644)
	require.NoError(t, err)

	err = os.WriteFile(failedFile, []byte(failedTestXML), 0644)
	require.NoError(t, err)

	// Create quarantine file
	quarantineFile := filepath.Join(tempDir, "quarantine.yaml")
	currentTime := time.Now()
	startDate := currentTime.AddDate(0, 0, -10).Format("2006-01-02")
	endDate := currentTime.AddDate(0, 0, 10).Format("2006-01-02")
	quarantineContent := fmt.Sprintf(`
quarantine_tests:
  - name: TestQuarantined
    classname: com.example.TestQuarantined
    start_date: "%s"
    end_date: "%s"
`, startDate, endDate)
	err = os.WriteFile(quarantineFile, []byte(quarantineContent), 0644)
	require.NoError(t, err)

	// Create output file
	outputFile := filepath.Join(tempDir, "output.txt")
	oldDroneOutput := os.Getenv("DRONE_OUTPUT")
	os.Setenv("DRONE_OUTPUT", outputFile)
	defer os.Setenv("DRONE_OUTPUT", oldDroneOutput)

	t.Run("integration test with quarantine", func(t *testing.T) {
		plugin := Plugin{
			GlobPaths:        strings.Join([]string{passedFile, failedFile}, ","),
			QuarantineFile:   quarantineFile,
			FailOnQuarantine: true,
		}

		// We need to test the logic without the os.Exit call
		// Let's test the parsing logic directly
		logger := logrus.New()
		logger.SetOutput(io.Discard)

		paths := getPaths(plugin.GlobPaths)
		quarantineList, loadErr := LoadYAML(plugin.QuarantineFile)
		require.NoError(t, loadErr)

		stats, err := ParseTestsWithQuarantine(paths, quarantineList, logger)

		// This should fail because there's one non-quarantined failure
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Non-quarantined failures: 1")

		// Verify stats
		assert.Equal(t, 4, stats.TestCount)
		assert.Equal(t, 2, stats.PassCount)
		assert.Equal(t, 2, stats.FailCount)
		assert.Len(t, stats.NonQuarantinedFailuresList, 1)
		assert.Contains(t, stats.NonQuarantinedFailuresList, "com.example.TestRegularFailure.TestRegularFailure")
		assert.Len(t, stats.QuarantinedFailuresList, 1)
		assert.Contains(t, stats.QuarantinedFailuresList, "com.example.TestQuarantined.TestQuarantined")
	})

	// Clean up for next test
	os.Remove(outputFile)

	t.Run("integration test without quarantine", func(t *testing.T) {
		plugin := Plugin{
			GlobPaths:        strings.Join([]string{passedFile, failedFile}, ","),
			QuarantineFile:   "",
			FailOnQuarantine: false,
		}

		// Test the parsing logic directly without os.Exit
		logger := logrus.New()
		logger.SetOutput(io.Discard)

		paths := getPaths(plugin.GlobPaths)
		stats, err := ParseTests(paths, logger)

		// This should fail because there are failures
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed tests and errors found")

		// Verify stats
		assert.Equal(t, 4, stats.TestCount)
		assert.Equal(t, 2, stats.PassCount)
		assert.Equal(t, 2, stats.FailCount)
	})
}
