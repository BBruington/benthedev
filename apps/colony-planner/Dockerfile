# The production image (spec §10, M6): one static binary, built without the devauth tag, with the SPA
# and the migrations embedded, running as non-root on distroless. Base images are pinned to the
# versions mise.toml pins, by tag and multi-platform digest; move the tag and digest together.
#
# No BuildKit cache mounts: Railway accepts one only with the service's own ID written into it
# (id=s/<service id>-...), so the dependency layers below are the cache instead.

FROM oven/bun:1.4.2@sha256:9114c058aeae42162ee16dd5084b95fe9473970bb6bcb5b232ab1630f0546895 AS web
WORKDIR /src/web
COPY web/package.json web/bun.lock web/bunfig.toml ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build

FROM golang:1.27.3-trixie@sha256:c65ecfe613b539ac74c62a4c3b2c27e8f8a6076384c8f27e8a3c0f3be17cc385 AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY db/ db/
COPY internal/ internal/
COPY web/embed.go web/
COPY --from=web /src/web/dist/spa web/dist/spa
RUN CGO_ENABLED=0 go build -trimpath -o /colony ./cmd/colony

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=server /colony /colony
# This image is only ever production: seed and reset refuse it, and serve listens on every interface.
ENV APP_ENV=production
USER nonroot:nonroot
ENTRYPOINT ["/colony"]
CMD ["serve"]
