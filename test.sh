#!/bin/sh

set -e

export GOV_TEST_POSTGRES_USERNAME=postgres
export GOV_TEST_POSTGRES_PASSWORD=admin

container_id=$(docker container run --detach --publish 5433:5432 --env POSTGRES_USER="$GOV_TEST_POSTGRES_USERNAME" --env POSTGRES_PASSWORD="$GOV_TEST_POSTGRES_PASSWORD" postgres:alpine)
echo "started container $container_id"

cleanup() {
  echo "cleaning up container $container_id"
  docker container stop "$container_id"
  docker container rm --force --volumes "$container_id"
  echo "cleaned up container $container_id"
}

trap cleanup EXIT

dsn=postgresql://$GOV_TEST_POSTGRES_USERNAME:$GOV_TEST_POSTGRES_PASSWORD@localhost:5433

until psql $dsn -A -t -c "select 'ok';" > /dev/null 2>&1; do
  echo "waiting for container $container_id"
  sleep 1
done

go test -trimpath -ldflags "-w -s" -race "$@"
