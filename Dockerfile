FROM scratch
MAINTAINER xorkevin
COPY bin/serve /
EXPOSE 8080
CMD ["/serve"]
