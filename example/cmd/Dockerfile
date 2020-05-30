FROM scratch
MAINTAINER xorkevin <kevin@xorkevin.com>
WORKDIR /home/governor
COPY bin/governor .
VOLUME /home/governor/config
VOLUME /home/governor/public
VOLUME /home/governor/templates
EXPOSE 8080
ENTRYPOINT ["/home/governor/governor"]
CMD ["serve"]
