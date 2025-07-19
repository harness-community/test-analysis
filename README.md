# Test Analysis

Harness plugin for parsing test reports. The plugin will exit with `exit status 1` if there are any tests failing in directories matching the input globs. The plugin currently only supports JUnit XML test reports.

## Build

Build the binary with the following commands:

```sh
$ go build
```

## Docker

Build the Docker image with the following commands:

```sh
$ docker buildx build -f docker/Dockerfile -t harnesscommunity/parse-test-reports:latest --platform=linux/amd64 --load .
```

## Usage

Execute from the working directory:
```sh
$ docker run -e PLUGIN_TEST_GLOBS="results.xml" harnesscommunity/parse-test-reports:latest
```

Execute the plugin in Harness pipeline:
```yaml
  - step:
      type: Plugin
      name: Parse Test Reports Plugin
      identifier: Parse_Test_Reports_Plugin
      spec:
        connectorRef: dockerConnector
        image: plugins/test-analysis:latest
        settings:
          test_globs: folder1/*.xml, folder2/*.xml
```

Below is the example with ‘fail_on_quarantine’ = true
```yaml
              - step:
                  type: Plugin
                  name: Test Analysis Plugin
                  identifier: Plugin_1
                  spec:
                    connectorRef: Plugins_Docker_Hub_Connector
                    image: plugins/test-analysis:latest
                    settings:
                      test_globs: sample1/*.xml, sample2/*.xml
                      quarantine_file: quarantinelist.yaml
                      fail_on_quarantine: true
              - step:
                  identifier: verify_output_variables
                  type: Run
                  name: Verify Output Variables
                  spec:
                    shell: Sh
                    command: |-
                      #!/bin/sh
                      echo "Test Analysis Plugin Results:"
                      echo "Total Tests: <+steps.Plugin_1.output.outputVariables.TOTAL_TESTS>"
                      echo "Passed Tests: <+steps.Plugin_1.output.outputVariables.PASSED_TESTS>"
                      echo "Failed Tests: <+steps.Plugin_1.output.outputVariables.FAILED_TESTS>"
                      echo "Skipped Tests: <+steps.Plugin_1.output.outputVariables.SKIPPED_TESTS>"
                      echo "Error Tests: <+steps.Plugin_1.output.outputVariables.ERROR_TESTS>"
```

## Running locally

```sh
$ PLUGIN_QUARANTINE_FILE='quarantinelist.yaml' PLUGIN_FAIL_ON_QUARANTINE=true DRONE_OUTPUT='OUTPUT_FILE.txt' PLUGIN_TEST_GLOBS='results.xml' ./parse-test-reports
```

### Running Tests

Run all tests:
```sh
$ go test ./...
```

Run with junit report:
```sh
$ TEST_RESULTS_DIR=test-results
$ JUNIT_REPORT_FILE=junit-report.xml
$ mkdir -p ${TEST_RESULTS_DIR}
$ go test -v ./... 2>&1 | go-junit-report -set-exit-code -out ${TEST_RESULTS_DIR}/${JUNIT_REPORT_FILE}
```

### Test Structure

- `parser_test.go` - Tests for parsing functions and quarantine logic
- `plugin_test.go` - Tests for the Plugin struct and its methods
