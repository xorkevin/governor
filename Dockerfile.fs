FROM scratch
MAINTAINER xorkevin
COPY bin/fsserve /
EXPOSE 3000
VOLUME /public
CMD ["/fsserve"]
