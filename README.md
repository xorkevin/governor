# governor

yet another microservice framework

import via `xorkevin.dev/governor`

#### confirm dev setup

- `make devup` will start a local docker network
- `make dev` will start a local golang server instance
- `curl localhost:8080/api/healthz/ping` should return back `Pong` and log `Ping` on the server
- `curl localhost:8080/api/healthz/check` should return back a timestamp and no errors
- `make devdown` will bring down the docker network

#### helpful tools

- `docker ps` will show all currently running containers
