FROM golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/server \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/outbox-relay ./cmd/outbox-relay \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/event-worker ./cmd/event-worker \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/token ./cmd/token

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app app

COPY --from=build /out/api /app/api
COPY --from=build /out/outbox-relay /app/outbox-relay
COPY --from=build /out/event-worker /app/event-worker
COPY --from=build /out/token /app/token

USER app
EXPOSE 8080
ENTRYPOINT ["/app/api"]
