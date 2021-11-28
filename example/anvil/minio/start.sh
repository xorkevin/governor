#!/bin/sh

set -e

export MINIO_ACCESS_KEY=$(cat /etc/miniopass/username.txt)
export MINIO_SECRET_KEY=$(cat /etc/miniopass/pass.txt)
minio server /var/lib/minio/data
