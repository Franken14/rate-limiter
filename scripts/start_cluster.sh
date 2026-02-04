#!/bin/bash

# Configuration
START_PORT=7000
NODES=6
HOST="127.0.0.1"
CLUSTER_DIR="cluster-test"

# Clean up previous run
echo "Cleaning up previous run..."
pkill redis-server
rm -rf $CLUSTER_DIR
mkdir -p $CLUSTER_DIR

echo "Starting $NODES Redis nodes..."

for ((i=0; i<NODES; i++)); do
  PORT=$((START_PORT + i))
  DIR="$CLUSTER_DIR/$PORT"
  mkdir -p $DIR
  
  cat <<EOF > $DIR/redis.conf
port $PORT
cluster-enabled yes
cluster-config-file nodes.conf
cluster-node-timeout 5000
appendonly yes
dir $DIR
daemonize yes
EOF
  
  echo "Starting node on port $PORT..."
  redis-server $DIR/redis.conf
done

echo "Waiting for nodes to start..."
sleep 2

echo "Creating cluster..."
# Construct the node list
NODE_LIST=""
for ((i=0; i<NODES; i++)); do
  PORT=$((START_PORT + i))
  NODE_LIST="$NODE_LIST $HOST:$PORT"
done

# Create the cluster (yes to force creation)
echo "yes" | redis-cli --cluster create $NODE_LIST --cluster-replicas 1

echo "Cluster started successfully!"
echo "Export this to run your app:"
echo "export REDIS_CLUSTER_ADDRS=\"$(echo $NODE_LIST | tr ' ' ',')\""
