package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

const junitFixture = `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="suite1" tests="4" failures="1" errors="1" skipped="1">
  <testcase classname="pkg.A" name="testPass" time="0.1"/>
  <testcase classname="pkg.A" name="testFail" time="0.1">
    <failure message="boom">boom</failure>
  </testcase>
  <testcase classname="pkg.A" name="testErr" time="0.1">
    <error message="kaboom">kaboom</error>
  </testcase>
  <testcase classname="pkg.A" name="testSkip" time="0.1">
    <skipped/>
  </testcase>
</testsuite>`

func writeFixture(t *testing.T) (dir, xmlPath string) {
	t.Helper()
	dir = t.TempDir()
	xmlPath = filepath.Join(dir, "TEST-sample.xml")
	if err := os.WriteFile(xmlPath, []byte(junitFixture), 0644); err != nil {
		t.Fatal(err)
	}
	return
}

func silentLog() *logrus.Logger {
	l := logrus.New()
	l.Out = os.NewFile(0, os.DevNull)
	return l
}

func TestGetPaths(t *testing.T) {
	got := getPaths("a/*.xml, b/*.xml")
	if len(got) != 2 || got[0] != "a/*.xml" || got[1] != "b/*.xml" {
		t.Fatalf("got %v", got)
	}
	if len(getPaths("")) != 0 {
		t.Fatal("empty input should yield no paths")
	}
}

func TestUniqueItems(t *testing.T) {
	got := uniqueItems([]string{"a", "b", "a", "c", "b"})
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("got %v", got)
	}
}

func TestExpandTilde(t *testing.T) {
	if p, _ := expandTilde(""); p != "" {
		t.Fatal("empty in -> empty out")
	}
	if p, _ := expandTilde("/abs"); p != "/abs" {
		t.Fatal("non-tilde unchanged")
	}
	p, err := expandTilde("~/x")
	if err != nil || !filepath.IsAbs(p) || !strings.HasSuffix(p, "/x") {
		t.Fatalf("got %q err=%v", p, err)
	}
	if _, err := expandTilde("~other/x"); err == nil {
		t.Fatal("expected error for ~user form")
	}
}

func TestIsURL(t *testing.T) {
	if !isURL("http://x") || !isURL("https://x") || isURL("/tmp/x") {
		t.Fatal("isURL wrong")
	}
}

func TestParseTests(t *testing.T) {
	_, xml := writeFixture(t)
	stats, err := ParseTests([]string{xml}, silentLog())
	if err == nil {
		t.Fatal("expected error due to failures/errors")
	}
	if stats.TestCount != 4 || stats.PassCount != 1 || stats.FailCount != 1 || stats.ErrorCount != 1 || stats.SkippedCount != 1 {
		t.Fatalf("bad stats: %+v", stats)
	}
}

func TestParseTestsNoMatch(t *testing.T) {
	if _, err := ParseTests([]string{filepath.Join(t.TempDir(), "nope-*.xml")}, silentLog()); err == nil {
		t.Fatal("expected error when no files match")
	}
}

func TestParseTestsWithQuarantine(t *testing.T) {
	_, xml := writeFixture(t)
	ql := map[string]interface{}{
		"quarantine_tests": []interface{}{
			map[interface{}]interface{}{
				"classname":  "pkg.A",
				"name":       "testFail",
				"start_date": "2024-01-01",
				"end_date":   "2099-01-01",
			},
		},
	}
	stats, err := ParseTestsWithQuarantine([]string{xml}, ql, silentLog())
	if err == nil {
		t.Fatal("expected error: testErr is not quarantined")
	}
	if stats.TestCount != 4 || stats.FailCount != 1 || stats.ErrorCount != 1 {
		t.Fatalf("bad stats: %+v", stats)
	}
}

func TestIsQuarantinedAndExpired(t *testing.T) {
	log := silentLog()
	ql := map[string]interface{}{
		"quarantine_tests": []interface{}{
			map[interface{}]interface{}{
				"classname":  "pkg.A",
				"name":       "testFail",
				"start_date": "2024-01-01",
				"end_date":   "2099-01-01",
			},
			map[interface{}]interface{}{
				"classname":  "pkg.A",
				"name":       "expiredOne",
				"start_date": "2000-01-01",
				"end_date":   "2000-12-31",
			},
		},
	}
	if !isQuarantined("pkg.A.testFail", ql, log) {
		t.Fatal("testFail should be quarantined")
	}
	if isQuarantined("pkg.A.unknown", ql, log) {
		t.Fatal("unknown should not be quarantined")
	}
	if !isExpired("pkg.A.expiredOne", ql, log) {
		t.Fatal("expiredOne should be expired")
	}
	if isExpired("pkg.A.testFail", ql, log) {
		t.Fatal("testFail is in range, not expired")
	}
}

func TestLoadYAMLLocal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "q.yaml")
	if err := os.WriteFile(p, []byte("quarantine_tests:\n  - classname: pkg.A\n    name: testFail\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadYAML(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m["quarantine_tests"]; !ok {
		t.Fatal("missing key")
	}
}

func TestWriteTestStatsNoOutput(t *testing.T) {
	t.Setenv("DRONE_OUTPUT", "")
	// should not panic; errors are logged
	writeTestStats(TestStats{TestCount: 1}, silentLog())
}

func TestWriteEnvToFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.env")
	t.Setenv("DRONE_OUTPUT", out)
	if err := WriteEnvToFile("FOO", "1", silentLog()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if !strings.Contains(string(b), "FOO=1") {
		t.Fatalf("got %q", b)
	}
}
