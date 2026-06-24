# syntax=docker/dockerfile:1.7
FROM golang:1.25.11-alpine3.23 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOFLAGS=-trimpath go build \
      -ldflags="-s -w" \
      -o /out/parse-test-reports .

FROM alpine:3.23.5

RUN apk update && apk add --no-cache libxml2-utils

COPY --from=build /out/parse-test-reports /usr/local/bin/parse-test-reports
RUN chmod +x /usr/local/bin/parse-test-reports

ENTRYPOINT ["/usr/local/bin/parse-test-reports"]
