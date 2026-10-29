# Garage, Homeloom's S3 store, on Alpine so it can set itself up on start (see entrypoint.sh).
FROM dxflrs/garage:v2.5.0 AS garage

FROM alpine:3.24
COPY --from=garage /garage /usr/local/bin/garage
COPY garage.toml /etc/garage.toml
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
VOLUME /var/lib/garage
EXPOSE 3900
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
