#!/bin/sh

. ./source.sh

namespace=governor

flightctl plan postgres -n $namespace -o base/postgres postgres
flightctl plan redis -n $namespace -o base/redis redis
flightctl plan minio -n $namespace -o base/minio minio
flightctl plan nats -n $namespace -o base/nats nats
flightctl plan natsstream -n $namespace -o base/natsstream natsstream
