FROM scratch
MAINTAINER xorkevin
WORKDIR /
COPY bin/governor .
COPY bin/govsetup .
VOLUME /config
VOLUME /public
VOLUME /templates
EXPOSE 8080
CMD ["/governor", "serve"]
