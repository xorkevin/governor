#!/bin/sh

set -e

export GOV_TEST_POSTGRES_USERNAME=postgres
export GOV_TEST_POSTGRES_PASSWORD=admin
export GOV_TEST_POSTGRES_DB=postgres
export GOV_TEST_POSTGRES_HOST=localhost
export GOV_TEST_POSTGRES_PORT=5433

container_id=$(docker container run --detach \
  --publish $GOV_TEST_POSTGRES_PORT:5432 \
  --env POSTGRES_USER="$GOV_TEST_POSTGRES_USERNAME" \
  --env POSTGRES_PASSWORD="$GOV_TEST_POSTGRES_PASSWORD" \
  --env POSTGRES_DB="$GOV_TEST_POSTGRES_DB" \
  --env PGDATA="/var/lib/postgresql/data/pgdata" \
  --env POSTGRES_HOST_AUTH_METHOD="scram-sha-256" \
  --env POSTGRES_INITDB_ARGS="--encoding UTF8 --locale=C --auth-local=trust --auth-host=scram-sha-256" \
  --env LANG="C" \
  postgres:alpine)
echo "started container $container_id"

cleanup() {
  echo "cleaning up container $container_id"
  docker container stop "$container_id"
  docker container rm --force --volumes "$container_id"
  echo "cleaned up container $container_id"
}

trap cleanup EXIT

dsn=postgresql://$GOV_TEST_POSTGRES_USERNAME:$GOV_TEST_POSTGRES_PASSWORD@$GOV_TEST_POSTGRES_HOST:$GOV_TEST_POSTGRES_PORT/$GOV_TEST_POSTGRES_DB

until psql $dsn -A -t -c "select 'ok';" > /dev/null 2>&1; do
  echo "waiting for container $container_id"
  sleep 1
done

go test -trimpath -ldflags "-w -s" -race "$@"
