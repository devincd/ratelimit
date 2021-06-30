#!/usr/bin/env bash

## starts with redis

#export LOG_LEVEL=debug
#export REDIS_SOCKET_TYPE=tcp
#export USE_STATSD=false
#export RUNTIME_ROOT=./data
#export RUNTIME_SUBDIRECTORY=ratelimit
#export RUNTIME_WATCH_ROOT=false
#export REDIS_URL=127.0.0.1:6379
#
#./bin/ratelimit

## starts with memory
export LOG_LEVEL=debug
export BACKEND_TYPE=memory
export MEMORY_CACHE_SIZE_IN_BYTES=1048576

export USE_STATSD=false
export RUNTIME_ROOT=./data
export RUNTIME_SUBDIRECTORY=ratelimit
export RUNTIME_WATCH_ROOT=false

./bin/ratelimit


## starts with memcached

