#syntax=docker/dockerfile:1.4
FROM mcr.microsoft.com/devcontainers/go:dev-1.26-trixie
RUN --mount=type=cache,target=/var/cache/apt --mount=type=cache,target=/var/cache/dpkg apt-get update && apt-get install -y --no-install-recommends dbus-test-runner dbus-bin libsecret-tools libsecret-1-dev
