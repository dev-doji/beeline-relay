# Beeline API container. Build context: beeline-api/.
# Migrations are embedded in the binary; secrets come from the environment at runtime.

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
# Render (and most hosts) set PORT; the API reads it and defaults to 8080.
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
