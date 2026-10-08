package node

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/go-connections/nat"
)

// OpenAllocatedPorts adds UFW rules for the host ports assigned to a server.
func OpenAllocatedPorts(ctx context.Context, bindings nat.PortMap) error {
	rules := make([]string, 0, len(bindings))
	for containerPort, entries := range bindings {
		protocol := containerPort.Proto()
		for _, binding := range entries {
			hostPort, err := strconv.Atoi(binding.HostPort)
			if err != nil || hostPort < 1 || hostPort > 65535 {
				return fmt.Errorf("open allocated ports: invalid host port %q", binding.HostPort)
			}
			rules = append(rules, fmt.Sprintf("%d/%s", hostPort, protocol))
		}
	}
	return addUFWRules(ctx, rules)
}

// OpenNodePortAllocations opens every configured node allocation for TCP and UDP.
func OpenNodePortAllocations(ctx context.Context, payload map[string]any) (map[string]any, error) {
	rawPorts, ok := payload["ports"].([]any)
	if !ok {
		return nil, fmt.Errorf("node port allocations are required")
	}
	ports := make([]int, 0, len(rawPorts))
	for _, raw := range rawPorts {
		port := number(raw, 0)
		if port < 1 || port > 65535 || port != float64(int(port)) {
			return nil, fmt.Errorf("invalid node port allocation %v", raw)
		}
		ports = append(ports, int(port))
	}
	ranges := portRangeRules(ports)
	rules := make([]string, 0, len(ranges)*2)
	for _, portRange := range ranges {
		rules = append(rules, portRange+"/tcp", portRange+"/udp")
	}
	if err := addUFWRules(ctx, rules); err != nil {
		return nil, err
	}
	return map[string]any{"opened": len(ports)}, nil
}

func portRangeRules(ports []int) []string {
	if len(ports) == 0 {
		return nil
	}
	sort.Ints(ports)
	rules := make([]string, 0, len(ports))
	start, end := ports[0], ports[0]
	for _, port := range ports[1:] {
		if port <= end {
			continue
		}
		if port == end+1 {
			end = port
			continue
		}
		rules = append(rules, formatPortRange(start, end))
		start, end = port, port
	}
	return append(rules, formatPortRange(start, end))
}

func formatPortRange(start, end int) string {
	if start == end {
		return strconv.Itoa(start)
	}
	return fmt.Sprintf("%d:%d", start, end)
}

func addUFWRules(ctx context.Context, rules []string) error {
	if runtime.GOOS != "linux" || len(rules) == 0 {
		return nil
	}
	ufwPath, err := exec.LookPath("ufw")
	if err != nil {
		return fmt.Errorf("open allocated ports: ufw is not installed on the node")
	}
	opened := make(map[string]bool, len(rules))
	for _, rule := range rules {
		if opened[rule] {
			continue
		}
		output, err := exec.CommandContext(ctx, ufwPath, "allow", rule).CombinedOutput()
		if err != nil {
			return fmt.Errorf("open UFW rule %s: %w: %s", rule, err, strings.TrimSpace(string(output)))
		}
		opened[rule] = true
	}
	return nil
}
