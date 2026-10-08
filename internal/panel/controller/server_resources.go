package controller

import (
	"context"
	"fmt"
	"math"

	"github.com/example/control-plane/internal/models"
	"github.com/example/control-plane/internal/resources"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	resourceMemoryMB int64 = 1024 * 1024
)

type nodeResourceCapacity struct {
	cpuCores     float64
	memoryMB     int64
	storageBytes int64
}

// nodeResourceUsage sums server limits, including every attached volume, for capacity reservations.
func (c *Controller) nodeResourceUsage(ctx context.Context, nodeID bson.ObjectID) (models.NodeResourceUsage, error) {
	var servers []models.Server
	if err := c.service.Servers.FindAll(ctx, bson.M{"nodeId": nodeID}, &servers); err != nil {
		return models.NodeResourceUsage{}, err
	}
	return aggregateNodeResourceUsage(servers), nil
}

// aggregateNodeResourceUsage counts each server's CPU, memory, disk, and attached volumes.
func aggregateNodeResourceUsage(servers []models.Server) models.NodeResourceUsage {
	var usage models.NodeResourceUsage
	for _, server := range servers {
		usage = addServerResourceUsage(usage, server)
	}
	return usage
}

// aggregateResourceUsageByNode groups resource reservations in one pass for the node list.
func aggregateResourceUsageByNode(servers []models.Server) map[bson.ObjectID]models.NodeResourceUsage {
	usage := make(map[bson.ObjectID]models.NodeResourceUsage)
	for _, server := range servers {
		usage[server.NodeID] = addServerResourceUsage(usage[server.NodeID], server)
	}
	return usage
}

// addServerResourceUsage adds a server's limits and disk-backed volume sizes to a reservation total.
func addServerResourceUsage(usage models.NodeResourceUsage, server models.Server) models.NodeResourceUsage {
	usage.CPUCores += math.Max(server.CPULimit, 0)
	usage.MemoryMB += max(server.MemoryMB, 0)
	usage.StorageBytes += max(server.DiskSizeBytes, 0)
	if server.DiskSizeBytes > 0 {
		usage.StorageBytes += resources.VolumeOverheadBytes
	}
	for _, volume := range server.Volumes {
		usage.StorageBytes += max(volume.SizeBytes, 0)
		if volume.SizeBytes > 0 {
			usage.StorageBytes += resources.VolumeOverheadBytes
		}
	}
	return usage
}

// availableNodeResources subtracts reserved limits and physical disk use from node telemetry.
func availableNodeResources(node models.Node, usage models.NodeResourceUsage) (nodeResourceCapacity, error) {
	if node.Stats.CPUThreads <= 0 || node.Stats.MemoryTotalBytes == 0 || node.Stats.StorageTotalBytes == 0 {
		return nodeResourceCapacity{}, fmt.Errorf("node has not reported complete CPU, memory, and storage capacity")
	}
	cpu := math.Max(float64(node.Stats.CPUThreads)-usage.CPUCores, 0)
	memoryTotalMB := int64(node.Stats.MemoryTotalBytes / uint64(resourceMemoryMB))
	memoryUsedMB := int64(node.Stats.MemoryUsedBytes / uint64(resourceMemoryMB))
	memory := min(max(memoryTotalMB-usage.MemoryMB, 0), max(memoryTotalMB-memoryUsedMB, 0))
	storageTotal := int64(min(node.Stats.StorageTotalBytes, uint64(math.MaxInt64)))
	storageUsed := int64(min(node.Stats.StorageUsedBytes, uint64(math.MaxInt64)))
	physicalFree := max(storageTotal-storageUsed, 0)
	reservedFree := max(storageTotal-usage.StorageBytes, 0)
	storage := min(physicalFree, reservedFree)
	return nodeResourceCapacity{cpuCores: cpu, memoryMB: memory, storageBytes: storage}, nil
}

// validateServerResources rejects requests above current unallocated node capacity.
func validateServerResources(node models.Node, usage models.NodeResourceUsage, cpu float64, memoryMB, diskBytes int64) error {
	capacity, err := availableNodeResources(node, usage)
	if err != nil {
		return err
	}
	if cpu < resources.CPUIncrementCores || math.Abs(cpu/resources.CPUIncrementCores-math.Round(cpu/resources.CPUIncrementCores)) > 1e-7 {
		return fmt.Errorf("CPU limit must use increments of 0.25 cores")
	}
	if memoryMB < resources.MemoryIncrementMB || memoryMB%resources.MemoryIncrementMB != 0 {
		return fmt.Errorf("memory must use increments of 256 MB")
	}
	if diskBytes < resources.StorageStepBytes || diskBytes%resources.StorageStepBytes != 0 {
		return fmt.Errorf("storage must use increments of 128 MB")
	}
	if cpu > capacity.cpuCores+1e-7 {
		return fmt.Errorf("requested %.2f CPU cores, but only %.2f cores are unallocated on this node", cpu, capacity.cpuCores)
	}
	if memoryMB > capacity.memoryMB {
		return fmt.Errorf("requested %d MB memory, but only %d MB are unallocated on this node", memoryMB, capacity.memoryMB)
	}
	if diskBytes+resources.VolumeOverheadBytes > capacity.storageBytes {
		availableDisk := max(capacity.storageBytes-resources.VolumeOverheadBytes, 0)
		return fmt.Errorf("requested %.3f GB storage, but only %.3f GB are available on this node", float64(diskBytes)/(1024*1024*1024), float64(availableDisk)/(1024*1024*1024))
	}
	return nil
}
