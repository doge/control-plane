package controller

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// nodeRegion detects a readable region label from the node's public IP address.
func (c *Controller) nodeRegion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	address := strings.TrimSpace(r.URL.Query().Get("address"))
	resolved, err := resolveAllIPs(address)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enter a public hostname or IP to detect its region"})
		return
	}
	var publicIP string
	for _, value := range resolved {
		ip := net.ParseIP(value)
		if ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
			publicIP = ip.String()
			break
		}
	}
	if publicIP == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "the address does not resolve to a public IP"})
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		"https://ipwho.is/"+url.PathEscape(publicIP)+"?fields=success,message,city,region,country", nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not prepare region lookup"})
		return
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "region lookup is temporarily unavailable"})
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "region lookup is temporarily unavailable"})
		return
	}
	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		City    string `json:"city"`
		Region  string `json:"region"`
		Country string `json:"country"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&result); err != nil || !result.Success {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not detect a region for this public IP"})
		return
	}
	parts := make([]string, 0, 3)
	seen := make(map[string]bool, 3)
	for _, part := range []string{result.City, result.Region, result.Country} {
		part = strings.TrimSpace(part)
		key := strings.ToLower(part)
		if part != "" && !seen[key] {
			parts = append(parts, part)
			seen[key] = true
		}
	}
	if len(parts) == 0 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "the lookup returned no region details"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"region": strings.Join(parts, ", ")})
}
