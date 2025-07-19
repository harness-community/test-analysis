package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/harness-community/parse-test-reports/gojunit"
	"github.com/mattn/go-zglob"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

func getPaths(globVal string) []string {
	paths := make([]string, 0)
	globValSplit := strings.Split(globVal, ",")
	for _, val := range globValSplit {
		if val == "" {
			continue
		}
		val = strings.TrimSpace(val)
		paths = append(paths, val)
	}
	return paths
}

// ParseTests parses XMLs and returns error if there are any failures
func ParseTests(paths []string, log *logrus.Logger) (TestStats, error) {
	files := getFiles(paths, log)
	stats := TestStats{}

	if len(files) == 0 {
		log.Errorln("could not find any files matching the provided report path")
		return stats, errors.New("could not find any files matching the provided report path")
	}

	for _, file := range files {
		suites, err := gojunit.IngestFile(file)
		if err != nil {
			log.WithError(err).WithField("file", file).Errorln("could not parse file")
			continue
		}
		fileStats := TestStats{}
		for _, suite := range suites {
			for _, test := range suite.Tests {
				fileStats.TestCount++
				switch test.Result.Status {
				case "passed":
					fileStats.PassCount++
				case "failed":
					fileStats.FailCount++
				case "skipped":
					fileStats.SkippedCount++
				case "error":
					fileStats.ErrorCount++
				}
			}
		}
		log.WithFields(logrus.Fields{
			"file":    file,
			"total":   fileStats.TestCount,
			"passed":  fileStats.PassCount,
			"failed":  fileStats.FailCount,
			"skipped": fileStats.SkippedCount,
			"errors":  fileStats.ErrorCount,
		}).Infoln("File processed")

		// Aggregate stats
		stats.TestCount += fileStats.TestCount
		stats.PassCount += fileStats.PassCount
		stats.FailCount += fileStats.FailCount
		stats.SkippedCount += fileStats.SkippedCount
		stats.ErrorCount += fileStats.ErrorCount
	}

	if stats.FailCount > 0 || stats.ErrorCount > 0 {
		return stats, errors.New("failed tests and errors found")
	}
	return stats, nil
}

// getFiles returns unique file paths after expanding the input paths
func getFiles(paths []string, log *logrus.Logger) []string {
	var files []string
	for _, p := range paths {
		path, err := expandTilde(p)
		if err != nil {
			log.WithError(err).WithField("path", p).Errorln("error expanding path")
			continue
		}
		matches, err := zglob.Glob(path)
		if err != nil {
			log.WithError(err).WithField("path", path).Errorln("error resolving path regex")
			continue
		}

		files = append(files, matches...)
	}
	return uniqueItems(files)
}

func uniqueItems(items []string) []string {
	var result []string
	set := make(map[string]bool)
	for _, item := range items {
		if !set[item] {
			result = append(result, item)
			set[item] = true
		}
	}
	return result
}

// expandTilde expands the given path to include the home directory if prefixed with `~`.
func expandTilde(path string) (string, error) {
	if path == "" {
		return path, nil
	}
	if path[0] != '~' {
		return path, nil
	}
	if len(path) > 1 && path[1] != '/' && path[1] != '\\' {
		return "", errors.New("cannot expand user-specific home dir")
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, path[1:]), nil
}

// LoadYAML reads a YAML file from either a URL or a local file
func LoadYAML(source string) (map[string]interface{}, error) {
	log := logrus.New()
	log.Infoln("Loading YAML from source:", source)

	var data []byte
	var err error

	if isURL(source) {
		resp, err := http.Get(source)
		if err != nil {
			log.WithError(err).Errorln("Failed to fetch YAML from URL")
			return nil, err
		}
		defer resp.Body.Close()

		data, err = io.ReadAll(resp.Body)
		if err != nil {
			log.WithError(err).Errorln("Failed to read YAML data from URL")
			return nil, err
		}
	} else {
		data, err = os.ReadFile(source)
		if err != nil {
			log.WithError(err).Errorln("Failed to read local YAML file")
			return nil, err
		}
	}

	var result map[string]interface{}
	err = yaml.Unmarshal(data, &result)
	if err != nil {
		log.WithError(err).Errorln("Failed to parse YAML")
		return nil, err
	}

	log.Infoln("Successfully loaded and parsed YAML")
	return result, nil
}

func isURL(source string) bool {
	return strings.HasPrefix(source, "http")
}

// ParseTestsWithQuarantine parses XMLs, considers quarantined tests, and returns errors if any non-quarantined failures are found
func ParseTestsWithQuarantine(paths []string, quarantineList map[string]interface{}, log *logrus.Logger) (TestStats, error) {
	files := getFiles(paths, log)
	stats := TestStats{}
	nonQuarantinedFailures := 0
	expiredTests := 0
	var nonQuarantinedFailuresList []string
	var expiredTestsList []string
	var quarantinedFailuresList []string

	if len(files) == 0 {
		log.Errorln("could not find any files matching the provided report path")
		return stats, errors.New("could not find any files matching the provided report path")
	}

	log.Infoln("Starting to parse tests with quarantine list")

	for _, file := range files {
		suites, err := gojunit.IngestFile(file)
		if err != nil {
			log.WithError(err).WithField("file", file).Errorln("could not parse file")
			continue
		}
		fileStats := TestStats{}
		for _, suite := range suites {
			for _, test := range suite.Tests {
				fileStats.TestCount++
				testIdentifier := test.Classname + "." + test.Name
				switch test.Result.Status {
				case "passed":
					fileStats.PassCount++
				case "failed", "error":
					if !isQuarantined(testIdentifier, quarantineList, log) {
						log.Infoln("Not Quarantined test failed:", testIdentifier)
						nonQuarantinedFailures++
						nonQuarantinedFailuresList = append(nonQuarantinedFailuresList, testIdentifier)
					} else if isExpired(testIdentifier, quarantineList, log) {
						log.Infoln("Quarantined test expired:", testIdentifier)
						expiredTests++
						expiredTestsList = append(expiredTestsList, testIdentifier)
					} else {
						quarantinedFailuresList = append(quarantinedFailuresList, testIdentifier)
					}

					if test.Result.Status == "failed" {
						fileStats.FailCount++
					} else {
						fileStats.ErrorCount++
					}
				case "skipped":
					fileStats.SkippedCount++
				}
			}
		}
		log.WithFields(logrus.Fields{
			"file":    file,
			"total":   fileStats.TestCount,
			"passed":  fileStats.PassCount,
			"failed":  fileStats.FailCount,
			"skipped": fileStats.SkippedCount,
			"errors":  fileStats.ErrorCount,
		}).Infoln("File processed")

		stats.TestCount += fileStats.TestCount
		stats.PassCount += fileStats.PassCount
		stats.FailCount += fileStats.FailCount
		stats.SkippedCount += fileStats.SkippedCount
		stats.ErrorCount += fileStats.ErrorCount
	}

	// Populate quarantine lists in stats
	stats.NonQuarantinedFailuresList = nonQuarantinedFailuresList
	stats.ExpiredTestsList = expiredTestsList
	stats.QuarantinedFailuresList = quarantinedFailuresList

	if nonQuarantinedFailures > 0 || expiredTests > 0 {
		// Construct the error message by concatenating string values
		errorMessage := "Non-quarantined failures: " + strconv.Itoa(nonQuarantinedFailures) +
			", Expired tests: " + strconv.Itoa(expiredTests) + " found"
		return stats, errors.New(errorMessage)
	}

	return stats, nil
}

func isQuarantined(testIdentifier string, quarantineList map[string]interface{}, log *logrus.Logger) bool {
	log.Infoln("Checking if test is quarantined:", testIdentifier)
	tests, ok := quarantineList["quarantine_tests"].([]interface{})
	if !ok {
		log.Warnln("Quarantine list invalid or missing 'quarantine_tests'")
		return false
	}
	for _, test := range tests {
		if testMap, ok := test.(map[interface{}]interface{}); ok {
			if quarantinedIdentifier, found := matchTestIdentifier(testMap, testIdentifier, log); found {
				log.Infoln("Test is quarantined:", quarantinedIdentifier)
				return true
			}
		}
	}
	log.Infoln("Test is not quarantined:", testIdentifier)
	return false
}

func isExpired(testIdentifier string, quarantineList map[string]interface{}, log *logrus.Logger) bool {
	tests, ok := quarantineList["quarantine_tests"].([]interface{})
	if !ok {
		log.Warnln("Quarantine list invalid or missing 'quarantine_tests'")
		return false
	}
	for _, test := range tests {
		if testMap, ok := test.(map[interface{}]interface{}); ok {
			if quarantinedIdentifier, found := matchTestIdentifier(testMap, testIdentifier, log); found {
				startDate, startOk := testMap["start_date"].(string)
				endDate, endOk := testMap["end_date"].(string)

				if startOk && endOk {
					currentDate := time.Now()

					startTime, err := time.Parse("2006-01-02", startDate)
					if err != nil {
						log.WithError(err).Warnln("Failed to parse start_date")
						continue
					}

					endTime, err := time.Parse("2006-01-02", endDate)
					if err != nil {
						log.WithError(err).Warnln("Failed to parse end_date")
						continue
					}

					if currentDate.Before(startTime) || currentDate.After(endTime) {
						log.WithFields(logrus.Fields{
							"test":        quarantinedIdentifier,
							"currentDate": currentDate,
							"startDate":   startTime,
							"endDate":     endTime,
						}).Infoln("Current Date lies outside start_date and end_date.")
						return true
					}
				}
			}
		}
	}

	log.Infoln("Test has no expiration set:", testIdentifier)
	return false
}

func matchTestIdentifier(testMap map[interface{}]interface{}, identifier string, log *logrus.Logger) (string, bool) {
	quarantinedClassname, classnameOk := testMap["classname"].(string)
	quarantinedName, nameOk := testMap["name"].(string)

	if classnameOk && nameOk {
		quarantinedIdentifier := quarantinedClassname + "." + quarantinedName
		if quarantinedIdentifier == identifier {
			log.Infoln("Test", identifier, "is quarantined")
			return quarantinedIdentifier, true
		}
	}
	return "", false
}
