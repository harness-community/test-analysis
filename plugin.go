package main

import (
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
)

type Plugin struct {
	GlobPaths        string
	QuarantineFile   string
	FailOnQuarantine bool
}

type TestStats struct {
	TestCount                  int
	FailCount                  int
	PassCount                  int
	SkippedCount               int
	ErrorCount                 int
	NonQuarantinedFailuresList []string
	ExpiredTestsList           []string
	QuarantinedFailuresList    []string
}

// Exec executes the plugin.
func (p Plugin) Exec() error {
	log := logrus.New()
	log.Out = os.Stdout

	if p.GlobPaths == "" {
		log.Errorf("%s plugin setting or %s environment variable is not set", globSetting, globEnv)
		os.Exit(1)
	}

	paths := getPaths(p.GlobPaths)
	log.Infof("Parsing test cases in globs: %s", paths)

	var stats TestStats
	var err error

	if p.FailOnQuarantine {
		if p.QuarantineFile == "" {
			log.Errorf("fail_on_quarantine is true, but %s plugin setting or %s environment variable is not set", quarantineFileSetting, quarantineFileEnv)
			os.Exit(1)
		}

		quarantineList, loadErr := LoadYAML(p.QuarantineFile)
		if loadErr != nil {
			log.Errorf("Error loading quarantine file: %s", loadErr)
			os.Exit(1)
		}

		stats, err = ParseTestsWithQuarantine(paths, quarantineList, log)
	} else {
		stats, err = ParseTests(paths, log)
	}

	// Always write output variables, even if there was an error
	writeTestStats(stats, log)

	log.Infof("Final test statistics: Total: %d, Passed: %d, Failed: %d, Skipped: %d, Errors: %d",
		stats.TestCount, stats.PassCount, stats.FailCount, stats.SkippedCount, stats.ErrorCount)

	log.Infof("nonQuarantinedFailures: %s", stats.NonQuarantinedFailuresList)
	log.Infof("expiredTests: %s", stats.ExpiredTestsList)
	log.Infof("quarantinedFailures: %s", stats.QuarantinedFailuresList)

	// Handle the error after writing stats
	if err != nil {
		log.Errorf("Error while parsing tests: %s", err)
		os.Exit(1)
	}

	return nil
}

func writeTestStats(stats TestStats, log *logrus.Logger) {
	statsMap := map[string]int{
		"TOTAL_TESTS":   stats.TestCount,
		"FAILED_TESTS":  stats.FailCount,
		"PASSED_TESTS":  stats.PassCount,
		"SKIPPED_TESTS": stats.SkippedCount,
		"ERROR_TESTS":   stats.ErrorCount,
	}

	for key, value := range statsMap {
		if err := WriteEnvToFile(key, strconv.Itoa(value), log); err != nil {
			log.Errorf("Error writing %s: %s", key, err)
		}
	}
}

func WriteEnvToFile(key, value string, log *logrus.Logger) error {
	outputFile, err := os.OpenFile(os.Getenv("DRONE_OUTPUT"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Errorf("Failed to open output file: %v", err)
		return err
	}
	defer outputFile.Close()

	log.Infof("Writing Test Stats %s : %s in func WriteEnvToFile to DRONE_OUTPUT", key, value)
	_, err = outputFile.WriteString(key + "=" + value + "\n")
	if err != nil {
		log.Errorf("Failed to write to env: %v", err)
		return err
	}
	return nil
}
