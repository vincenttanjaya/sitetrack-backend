FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sitetrack-api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sitetrack-seed ./cmd/seed

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S app \
 && adduser -S -G app app
COPY --from=build /out/sitetrack-api /out/sitetrack-seed /usr/local/bin/
USER app
EXPOSE 8080
CMD ["sitetrack-api"]
