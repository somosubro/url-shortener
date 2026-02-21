# Build stage
FROM golang:1.26 AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o url-shortener ./cmd/url-shortener

# Run stage (small, secure-ish)
FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /app/url-shortener /url-shortener

EXPOSE 8080
ENTRYPOINT ["/url-shortener"]