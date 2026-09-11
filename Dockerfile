FROM golang:1.26.8-alpine AS build

WORKDIR /src

COPY receiver/zabbixreceiver/go.mod receiver/zabbixreceiver/go.sum ./receiver/zabbixreceiver/
COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-w" -o /out/otelcol-zabbix ./cmd/otelcol-zabbix

FROM gcr.io/distroless/static-debian13:nonroot

COPY --chown=10001:10001 --from=build /out/otelcol-zabbix /otelcol-zabbix
COPY --chown=10001:10001 configs/otelcol.yaml /etc/otelcol-zabbix/config.yaml

USER 10001:10001

EXPOSE 13133 8888

ENTRYPOINT ["/otelcol-zabbix"]
CMD ["--config=/etc/otelcol-zabbix/config.yaml"]
