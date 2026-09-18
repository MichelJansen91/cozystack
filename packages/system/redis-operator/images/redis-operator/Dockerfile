FROM --platform=$BUILDPLATFORM golang:1.27@sha256:3680233e3204827fbdc66088528ae6d4b3d034f51d03a99d454f6de034888244 AS builder

ARG VERSION=v3.3.5

ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

RUN curl -sSL https://github.com/freshworks-oss/redis-operator/archive/refs/tags/${VERSION}.tar.gz | tar -xzvf- --strip=1

# The carried TLS patch is source only: its CRD half ships as the chart
# patch, so the tarball's CRD and chart copies stay at v3.3.5 and the tests
# that read them would fail instead of skipping.
RUN rm -rf manifests charts example .github

COPY patches /patches
RUN git apply /patches/*.diff

# go build never compiles a _test.go, so nothing else runs the tests of the
# packages the patches touch. Each package is tested from its own directory:
# that finds the module it belongs to, and an empty list runs nothing.
RUN for dir in $(git apply --numstat /patches/*.diff | cut -f3 | grep '\.go$' | xargs -n1 dirname | sort -u); do \
      (cd "$dir" && go test .) || exit 1; \
    done

# The integration tests the patches edit sit behind a build tag and need a
# cluster, so they are only type-checked here.
RUN go vet -tags integration ./test/integration/redisfailover

RUN GOOS=$TARGETOS GOARCH=$TARGETARCH VERSION=$VERSION ./scripts/build.sh

FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
RUN apk --no-cache add \
  ca-certificates
COPY --from=builder /workspace/bin/redis-operator /usr/local/bin
RUN addgroup -g 1000 rf && \
  adduser -D -u 1000 -G rf rf && \
  chown rf:rf /usr/local/bin/redis-operator
USER rf

ENTRYPOINT ["/usr/local/bin/redis-operator"]

