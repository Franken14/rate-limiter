#!/bin/bash

CLUSTER_DIR="cluster-test"

echo "Stopping Redis nodes..."
pkill -f "redis-server *:700"

echo "Cleaning up directory..."
rm -rf $CLUSTER_DIR

echo "Local cluster stopped."
