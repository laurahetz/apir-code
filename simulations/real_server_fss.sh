#!/bin/bash

id=$1
scheme=$2
expriment=$3

export GOGC=8000

# remove stats log and create new files
rm results/stats*

# build server
cd ../cmd/grpc/server
go build

# go back to simultion directory
cd - > /dev/null

# move to root
cd ../

# run servers
echo "##### running server $id with ${SCHEME} scheme #####"
# run server given the correct scheme 
cmd/grpc/server/server -id=$id -files=31 -experiment -scheme=${SCHEME} | tee -a simulations/results/stats_server-${id}_${SCHEME}_${TARGET}.log
wait $!
