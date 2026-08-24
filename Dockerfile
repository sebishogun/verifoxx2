# syntax=docker/dockerfile:1
# Multi-stage build: a pinned Go 1.27.0 Debian builder runs the bounded test
# suite and produces a static binary; a scratch runtime carries only the
# binary, policy, and fixtures under a numeric non-root user.

FROM golang:1.27.0-bookworm AS builder

WORKDIR /src

# Only what the tests and build need; .dockerignore keeps .git, bin, .tools,
# docs, scripts, and editor artifacts out of the context.
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY policies ./policies
COPY fixtures ./fixtures
COPY results ./results

RUN go test -count=1 -timeout 60s ./...
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/verifoxx ./cmd/verifoxx

FROM scratch

WORKDIR /app
COPY --from=builder /out/verifoxx /app/verifoxx
COPY --from=builder /src/policies /app/policies
COPY --from=builder /src/fixtures /app/fixtures

# Numeric non-root user (UID/GID 65532); scratch has no passwd file.
USER 65532:65532

ENTRYPOINT ["/app/verifoxx"]
# Default: evaluate the supplied pack with --output - so stdout carries only
# the JSON pack and the human table goes to stderr.
CMD ["--policy", "policies/policy.json", "--requests", "fixtures/requests.json", "--evidence", "fixtures/evidence.json", "--output", "-"]
