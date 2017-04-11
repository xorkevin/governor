# governor
internal org management tooling

#### confirm dev setup

- `make dev` will start a local golang server instance
- `curl localhost:8080/api/health/ping` should return back `Pong` and log `Ping` on the server


#### confirm docker setup

- `make docker` will start two development servers locally
- `curl localhost:8080/api/health/check` should return back the time


#### confirm pg setup

- `make pg-setup` will setup a local postgres docker volume
- `make pg-run` will start a local postgres instance


#### helpful tools

- `docker ps` will show all currently running containers
