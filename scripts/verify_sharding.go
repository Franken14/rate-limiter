package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/redis/go-redis/v9"
)

func main() {
	clusterAddrs := os.Getenv("REDIS_CLUSTER_ADDRS")
	if clusterAddrs == "" {
		// Default to the local cluster ports from our docker/script setup
		clusterAddrs = "127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002,127.0.0.1:7003,127.0.0.1:7004,127.0.0.1:7005"
	}

	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: strings.Split(clusterAddrs, ","),
	})
	defer rdb.Close()

	ctx := context.Background()
	distribution := make(map[string]int)

	fmt.Println("Generating 1000 random keys to check sharding...")

	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("user:%d", i)

		// We use CLUSTER KEYSLOT to verify, or simpler: just ask Redis where this key goes
		// In a real cluster client, we can find the master for a key.
		// Go-redis abstraction hides this, so we will just SET it and see if it works,
		// and maybe check Client info if possible.

		// Actually, let's just use the client to get the slot.
		slot := int(rdb.ClusterKeySlot(ctx, key).Val())

		// Map slot to a "shard" bucket (approximate, since we have 3 masters usually)
		// Slots 0-5460, 5461-10922, 10923-16383
		shard := "Unknown"
		if slot < 5461 {
			shard = "Shard-1 (Slots 0-5460)"
		} else if slot < 10923 {
			shard = "Shard-2 (Slots 5461-10922)"
		} else {
			shard = "Shard-3 (Slots 10923-16383)"
		}

		distribution[shard]++
	}

	fmt.Println("\nKey Distribution Results:")
	for shard, count := range distribution {
		fmt.Printf("%s: %d keys\n", shard, count)
	}
}
