# Multi-stage build for LumoNAS
FROM golang:1.26-alpine AS go-builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

RUN CGO_ENABLED=1 go build -o /build/lumonasd ./cmd/lumonasd/
RUN CGO_ENABLED=1 go build -o /build/lumonas-web ./cmd/lumonas-web/
RUN CGO_ENABLED=1 go build -o /build/lumonas-privd ./cmd/lumonas-privd/

FROM node:22-alpine AS node-builder

WORKDIR /build
COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm build

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata openssl

RUN addgroup -S lumonas && adduser -S -G lumonas lumonas

COPY --from=go-builder /build/lumonasd /build/lumonas-web /build/lumonas-privd /usr/local/bin/
COPY --from=node-builder /build/dist /usr/share/lumonas/web
COPY catalog/ /usr/share/lumonas/catalog/

RUN mkdir -p /etc/lumonas /var/lib/lumonas /srv/lumonas/docker/stacks /run/lumonas
RUN chown -R lumonas:lumonas /etc/lumonas /var/lib/lumonas /srv/lumonas /run/lumonas

EXPOSE 8080 8081

ENTRYPOINT ["lumonasd"]
CMD ["-listen", "0.0.0.0:8080"]
