# syntax=docker/dockerfile:1

# --- build stage -------------------------------------------------------
FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 produces a fully static binary so it runs on the distroless
# static base image below with no libc dependency.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sitewatch .

# --- runtime stage -------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /etc/sitewatch

COPY --from=build /out/sitewatch /usr/local/bin/sitewatch

# Bind-mount your own sitewatch.yaml to /etc/sitewatch/sitewatch.yaml, and
# a volume at /etc/sitewatch for the history file to persist across
# container restarts, e.g.:
#   docker run -v ./sitewatch.yaml:/etc/sitewatch/sitewatch.yaml \
#              -v sitewatch-data:/etc/sitewatch \
#              -e SITEWATCH_SMTP_PASSWORD \
#              ghcr.io/ricky1800/sitewatch:latest

ENTRYPOINT ["/usr/local/bin/sitewatch"]
CMD ["run", "--config", "/etc/sitewatch/sitewatch.yaml"]
