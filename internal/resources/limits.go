// Package resources defines allocation steps shared by the panel and node agent.
package resources

const (
	CPUIncrementCores   float64 = 0.25
	MemoryIncrementMB   int64   = 256
	StorageStepBytes    int64   = 128 * 1024 * 1024
	VolumeOverheadBytes int64   = 128 * 1024 * 1024
	MinimumVolumeBytes          = StorageStepBytes
)
