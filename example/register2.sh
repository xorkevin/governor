#!/bin/sh

. ./source.sh

namespace=governor

flightctl register postgres -n $namespace -e postgres
