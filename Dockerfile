# Build stage
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
# Allow the Go toolchain to satisfy any version directive in go.mod.
ENV GOTOOLCHAIN=auto
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /courtyard ./cmd/courtyard

# Runtime stage — minimal attack surface
FROM gcr.io/distroless/static
COPY --from=build /courtyard /courtyard
ENTRYPOINT ["/courtyard"]
