FROM alpine:3 as builder
RUN apk add --no-cache ca-certificates tzdata mailcap

FROM scratch
MAINTAINER xorkevin <kevin@xorkevin.com>
COPY --link --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --link --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --link --from=builder /etc/mime.types /etc/mime.types
COPY --link bin/governor /home/governor/governor
EXPOSE 8080
WORKDIR /home/governor
ENTRYPOINT ["/home/governor/governor"]
CMD ["serve"]
