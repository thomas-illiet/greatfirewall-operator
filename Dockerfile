# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.26.3 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /out/operator ./cmd/operator
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /out/mock-api ./cmd/mock-api

FROM gcr.io/distroless/static-debian12:nonroot AS operator
COPY --from=build /out/operator /operator
ENTRYPOINT ["/operator"]

FROM gcr.io/distroless/static-debian12:nonroot AS mock-api
COPY --from=build /out/mock-api /mock-api
ENTRYPOINT ["/mock-api"]
