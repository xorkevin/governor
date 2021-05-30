#!/bin/sh

set -e

export MINIO_ACCESS_KEY=admin
export MINIO_SECRET_KEY=$(cat /etc/miniopass/pass.txt)
minio server /var/lib/minio/data
