FROM alpine:3 as builder
RUN apk add --no-cache ca-certificates tzdata mailcap

FROM scratch
MAINTAINER xorkevin <kevin@xorkevin.com>
WORKDIR /home/governor
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/
COPY --from=builder /etc/mime.types /etc/
COPY bin/governor .
VOLUME /home/governor/config
VOLUME /home/governor/public
VOLUME /home/governor/templates
EXPOSE 8080
ENTRYPOINT ["/home/governor/governor"]
CMD ["serve"]
