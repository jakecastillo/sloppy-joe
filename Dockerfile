# Build static sloppyd + sloppy, ship on distroless.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/sloppyd ./cmd/sloppyd \
 && CGO_ENABLED=0 go build -trimpath -o /out/sloppy ./cmd/sloppy

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/sloppyd /out/sloppy /usr/local/bin/
COPY --from=build /src/examples /app/examples
EXPOSE 8723
ENTRYPOINT ["sloppyd"]
