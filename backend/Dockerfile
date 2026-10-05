FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /platform ./cmd/platform

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
USER app
COPY --from=build /platform /usr/local/bin/platform
EXPOSE 8080
ENTRYPOINT ["platform"]
CMD ["serve"]
