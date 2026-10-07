# Frontend build
FROM node:22-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ .
RUN npm run build

# Go build
FROM golang:1.26-bookworm AS go-build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
COPY --from=web-build /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -o /out/control-plane-panel ./cmd/panel
RUN CGO_ENABLED=0 go build -o /out/control-plane-node ./cmd/node

FROM debian:bookworm-slim
RUN useradd -r -u 10001 control-plane
WORKDIR /app
COPY --from=go-build /out/control-plane-panel /app/control-plane-panel
COPY --from=go-build /out/control-plane-node /app/control-plane-node
COPY --from=go-build /src/web/dist /app/web/dist
USER control-plane
EXPOSE 8080
ENTRYPOINT ["/app/control-plane-panel"]
