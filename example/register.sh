#!/bin/sh

namespace=governor

flightctl register postgres -n $namespace postgres
flightctl register redis -n $namespace redis
flightctl register minio -n $namespace minio
flightctl register nats -n $namespace nats
flightctl register natsstream -n $namespace natsstream
