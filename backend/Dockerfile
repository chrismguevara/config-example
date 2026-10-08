# Build from the repository root: docker build -f backend/Dockerfile .
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY schemas ./schemas
COPY backend ./backend
RUN CGO_ENABLED=0 go build -o /out/api ./backend/cmd/api

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
