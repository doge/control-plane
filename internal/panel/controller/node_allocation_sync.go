package controller

import (
	"context"
	"net"

	"github.com/example/control-plane/internal/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// reconcileNodeBindAllocations moves a fresh node's ports onto detected interfaces when its saved IPs are not bindable.
func (a *App) reconcileNodeBindAllocations(ctx context.Context, node models.Node, bindIPs []string) {
	usable := make([]string, 0, len(bindIPs))
	usableSet := make(map[string]bool, len(bindIPs))
	for _, value := range bindIPs {
		ip := net.ParseIP(value)
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			continue
		}
		canonical := ip.String()
		if !usableSet[canonical] {
			usable = append(usable, canonical)
			usableSet[canonical] = true
		}
	}
	if len(usable) == 0 || len(node.PortAllocations) == 0 {
		return
	}
	for _, allocation := range node.PortAllocations {
		if ip := net.ParseIP(allocation.IP); ip != nil && usableSet[ip.String()] {
			return
		}
	}
	count, err := a.db.Collection("servers").CountDocuments(ctx, bson.M{"nodeId": node.ID})
	if err != nil || count > 0 {
		return
	}
	ports := make([]int, 0, len(node.PortAllocations))
	seenPorts := make(map[int]bool, len(node.PortAllocations))
	for _, allocation := range node.PortAllocations {
		if allocation.Port > 0 && allocation.Port <= 65535 && !seenPorts[allocation.Port] {
			ports = append(ports, allocation.Port)
			seenPorts[allocation.Port] = true
		}
	}
	if len(ports) == 0 {
		return
	}
	allocations := make([]models.NodePortAllocation, 0, len(usable)*len(ports))
	for _, ip := range usable {
		for _, port := range ports {
			allocations = append(allocations, models.NodePortAllocation{IP: ip, Port: port})
		}
	}
	_, _ = a.db.Collection("nodes").UpdateOne(ctx, bson.M{"_id": node.ID}, bson.M{"$set": bson.M{
		"ips": usable, "portAllocations": allocations,
	}})
}
