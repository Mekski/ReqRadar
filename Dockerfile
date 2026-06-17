# Single multi-stage Dockerfile for all Go services. The build arg SERVICE picks
# which cmd/ to compile, so collector/processor/api/migrate/seed all ship from the
# same image definition (and CI can build them in a matrix). Migrations and the
# seed watchlist are embedded, so the runtime image needs no loose files.
#
#   docker build --build-arg SERVICE=api -t reqradar-api .
ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
# Cache module downloads on their own layer.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG SERVICE
RUN test -n "$SERVICE" || (echo "SERVICE build-arg is required" && exit 1)
# Static binary so it runs in a scratch/distroless-style runtime.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${SERVICE}

# Distroless static: no shell, no package manager — small attack surface. Includes
# CA certs (needed for HTTPS to the ATS APIs, Telegram, and Gemini) and tzdata.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
# Defaults assume the compose network (service hostnames); override via env.
ENV REQRADAR_NATS_URL=nats://nats:4222 \
    REQRADAR_POSTGRES_DSN=postgres://reqradar:reqradar@postgres:5432/reqradar?sslmode=disable
ENTRYPOINT ["/app"]
