FROM golang:1.26.3-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/parse-test-reports .

FROM alpine:latest

RUN apk update && apk add --no-cache libxml2-utils

COPY --from=build /out/parse-test-reports /usr/local/bin/parse-test-reports
RUN chmod +x /usr/local/bin/parse-test-reports

ENTRYPOINT ["/usr/local/bin/parse-test-reports"]
