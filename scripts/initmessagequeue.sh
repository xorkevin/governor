#!/usr/bin/env bash

# 1: password

PGPASSWORD=$1 psql -h localhost -p 5433 -U nss -d nss_db -a -f scripts/initmessagequeue.sql
