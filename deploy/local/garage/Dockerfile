FROM dxflrs/amd64_garage:v2.2.0 AS garage

FROM alpine:3.20
COPY --from=garage /garage /garage
