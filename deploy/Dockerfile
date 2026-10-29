FROM docker.io/library/node:24.15.0-bookworm-slim AS web
ARG NPM_REGISTRY=https://registry.npmjs.org
RUN npm install --global pnpm@11.2.2 --registry="${NPM_REGISTRY}"
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN --mount=type=cache,id=pt-pnpm-store,target=/pnpm-store \
    pnpm install --frozen-lockfile --store-dir /pnpm-store --registry="${NPM_REGISTRY}"
COPY web/ ./
RUN pnpm build

FROM docker.io/library/golang:1.27.0-bookworm AS go
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY migrations/ ./migrations/
COPY --from=web /src/web/dist/ ./internal/web/dist/
ARG VERSION=0.0.0-dev
RUN --mount=type=cache,id=pt-go-build,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" -o /out/pandatable ./cmd/pandatable

FROM docker.io/library/debian:bookworm-slim AS pgdg-key
ARG APT_MIRROR=http://deb.debian.org/debian
RUN sed -i "s|http://deb.debian.org/debian|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources \
    && apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && curl --fail --silent --show-error --location https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /pgdg.asc

FROM docker.io/library/debian:bookworm-slim AS runtime
ARG APT_MIRROR=http://deb.debian.org/debian
ARG PGDG_MIRROR=https://apt.postgresql.org/pub/repos/apt
COPY --from=pgdg-key /pgdg.asc /usr/share/keyrings/pgdg.asc
RUN sed -i "s|http://deb.debian.org/debian|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources \
    && apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && echo "deb [signed-by=/usr/share/keyrings/pgdg.asc] ${PGDG_MIRROR} bookworm-pgdg main" > /etc/apt/sources.list.d/pgdg.list \
    && apt-get update && apt-get install -y --no-install-recommends postgresql-client-18 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 pandatable \
    && useradd --uid 10001 --gid 10001 --no-create-home --home-dir /data --shell /usr/sbin/nologin pandatable \
    && mkdir -p /data/files /data/backups /data/tmp \
    && chown -R 10001:10001 /data
ENV TZ=UTC
USER 10001:10001
WORKDIR /data
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=60s --retries=3 CMD ["pandatable", "healthcheck"]
ENTRYPOINT ["pandatable"]
CMD ["serve"]

# Task-level E2E only (D-158): host-built `ptdev build --linux` binary on the same runtime.
FROM runtime AS prebuilt
COPY --chmod=0755 bin/pandatable-linux-amd64 /usr/local/bin/pandatable

# Release image: must stay the last stage so the default target is unchanged.
FROM runtime
COPY --from=go /out/pandatable /usr/local/bin/pandatable
