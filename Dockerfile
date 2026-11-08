FROM --platform=$BUILDPLATFORM node:24-alpine AS assets
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY internal/web/assets ./internal/web/assets
RUN npm run build && npm run test:js

FROM --platform=$BUILDPLATFORM golang:1.26.5 AS build
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=assets /src/internal/web/assets/dist ./internal/web/assets/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /out/library ./cmd/server

FROM build AS test
RUN go test ./... && go vet ./...

FROM --platform=$BUILDPLATFORM golang:1.26.5 AS migrations
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /goose
RUN go mod init library.local/migrations && go get github.com/pressly/goose/v3/cmd/goose@v3.27.3
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -tags='no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb' \
    -o /out/goose github.com/pressly/goose/v3/cmd/goose

FROM alpine:3.23
RUN apk add --no-cache ca-certificates
LABEL org.opencontainers.image.source="https://github.com/ajorgensen/ourlibrary"
COPY --from=build /out/library /usr/local/bin/library
COPY --from=migrations /out/goose /usr/local/bin/goose
COPY internal/database/migrations /migrations
COPY scripts/migrate /usr/local/bin/migrate
USER 65532:65532
ENV HTTP_ADDRESS=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/library"]
