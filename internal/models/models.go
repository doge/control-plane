package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type User struct {
	ID                 bson.ObjectID   `bson:"_id,omitempty" json:"id"`
	Username           string          `bson:"username" json:"username"`
	Email              string          `bson:"email" json:"email"`
	PasswordHash       string          `bson:"passwordHash" json:"-"`
	TOTPSecret         string          `bson:"totpSecret,omitempty" json:"-"`
	TOTPEnabled        bool            `bson:"totpEnabled" json:"totpEnabled"`
	TOTPRecoveryHashes []string        `bson:"totpRecoveryHashes,omitempty" json:"-"`
	TOTPSetupSecret    string          `bson:"totpSetupSecret,omitempty" json:"-"`
	TOTPSetupExpiresAt time.Time       `bson:"totpSetupExpiresAt,omitempty" json:"-"`
	Role               string          `bson:"role" json:"role"`
	Permissions        []string        `bson:"-" json:"permissions,omitempty"`
	ManagedServers     []ManagedServer `bson:"-" json:"managedServers,omitempty"`
	Disabled           bool            `bson:"disabled" json:"disabled"`
	CreatedAt          time.Time       `bson:"createdAt" json:"createdAt"`
}

type ManagedServer struct {
	ID       bson.ObjectID `json:"id"`
	Name     string        `json:"name"`
	Status   string        `json:"status"`
	NodeID   bson.ObjectID `json:"nodeId"`
	ConfigID bson.ObjectID `json:"configId"`
}

type Role struct {
	ID          string    `bson:"_id" json:"id"`
	Name        string    `bson:"name" json:"name"`
	Description string    `bson:"description" json:"description"`
	Permissions []string  `bson:"permissions" json:"permissions"`
	CreatedAt   time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time `bson:"updatedAt" json:"updatedAt"`
}

type Session struct {
	ID        string        `bson:"_id"`
	UserID    bson.ObjectID `bson:"userId"`
	ExpiresAt time.Time     `bson:"expiresAt"`
}

type Node struct {
	ID              bson.ObjectID        `bson:"_id,omitempty" json:"id"`
	Name            string               `bson:"name" json:"name"`
	Region          string               `bson:"region" json:"region"`
	Address         string               `bson:"address" json:"address"`
	IPs             []string             `bson:"ips,omitempty" json:"ips,omitempty"`
	PortAllocations []NodePortAllocation `bson:"portAllocations,omitempty" json:"portAllocations,omitempty"`
	Stats           NodeStats            `bson:"stats,omitempty" json:"stats"`
	TokenHash       string               `bson:"tokenHash" json:"-"`
	Status          string               `bson:"status" json:"status"`
	LastSeen        time.Time            `bson:"lastSeen" json:"lastSeen"`
	CreatedAt       time.Time            `bson:"createdAt" json:"createdAt"`
}

type NodePortAllocation struct {
	IP   string `bson:"ip" json:"ip"`
	Port int    `bson:"port" json:"port"`
}

type NodeStats struct {
	CPUThreads        int       `bson:"cpuThreads" json:"cpuThreads"`
	Architecture      string    `bson:"architecture" json:"architecture"`
	Kernel            string    `bson:"kernel" json:"kernel"`
	CPUPercent        float64   `bson:"cpuPercent" json:"cpuPercent"`
	MemoryUsedBytes   uint64    `bson:"memoryUsedBytes" json:"memoryUsedBytes"`
	MemoryTotalBytes  uint64    `bson:"memoryTotalBytes" json:"memoryTotalBytes"`
	StorageUsedBytes  uint64    `bson:"storageUsedBytes" json:"storageUsedBytes"`
	StorageTotalBytes uint64    `bson:"storageTotalBytes" json:"storageTotalBytes"`
	UpdatedAt         time.Time `bson:"updatedAt" json:"updatedAt"`
}

type Config struct {
	ID          bson.ObjectID `bson:"_id,omitempty" json:"id"`
	Name        string        `bson:"name" json:"name"`
	Slug        string        `bson:"slug" json:"slug"`
	Description string        `bson:"description" json:"description"`
	Author      string        `bson:"author,omitempty" json:"author,omitempty"`
	Spec        ConfigSpec    `bson:"spec" json:"spec"`
	CreatedAt   time.Time     `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time     `bson:"updatedAt" json:"updatedAt"`
}

type ConfigSpec struct {
	Version                int               `bson:"version" json:"version"`
	DockerImages           []string          `bson:"dockerImages" json:"dockerImages"`
	Startup                string            `bson:"startup" json:"startup"`
	Stop                   string            `bson:"stop,omitempty" json:"stop,omitempty"`
	User                   string            `bson:"user,omitempty" json:"user,omitempty"`
	WorkingDirectory       string            `bson:"workingDirectory,omitempty" json:"workingDirectory,omitempty"`
	Install                *ConfigInstall    `bson:"install,omitempty" json:"install,omitempty"`
	Environment            map[string]string `bson:"environment,omitempty" json:"environment,omitempty"`
	Variables              []ConfigVariable  `bson:"variables,omitempty" json:"variables,omitempty"`
	Ports                  []ConfigPort      `bson:"ports,omitempty" json:"ports,omitempty"`
	DataDirectory          string            `bson:"dataDirectory,omitempty" json:"dataDirectory,omitempty"`
	Resources              ConfigResources   `bson:"resources,omitempty" json:"resources,omitempty"`
	SupportedArchitectures []string          `bson:"supportedArchitectures,omitempty" json:"supportedArchitectures,omitempty"`
}

type ConfigInstall struct {
	Image      string `bson:"image" json:"image"`
	Entrypoint string `bson:"entrypoint,omitempty" json:"entrypoint,omitempty"`
	Script     string `bson:"script" json:"script"`
}

type ConfigVariable struct {
	Name        string `bson:"name" json:"name"`
	Type        string `bson:"type,omitempty" json:"type,omitempty"`
	Default     string `bson:"default,omitempty" json:"default,omitempty"`
	Required    bool   `bson:"required,omitempty" json:"required,omitempty"`
	UI          bool   `bson:"ui,omitempty" json:"ui,omitempty"`
	Secret      bool   `bson:"secret,omitempty" json:"secret,omitempty"`
	Description string `bson:"description,omitempty" json:"description,omitempty"`
}

type ConfigPort struct {
	Name      string `bson:"name" json:"name"`
	Container int    `bson:"container" json:"container"`
	Protocol  string `bson:"protocol" json:"protocol"`
}

type ConfigResources struct {
	CPULimit float64 `bson:"cpuLimit,omitempty" json:"cpuLimit,omitempty"`
	MemoryMB int64   `bson:"memoryMB,omitempty" json:"memoryMB,omitempty"`
}

type Server struct {
	ID            bson.ObjectID     `bson:"_id,omitempty" json:"id"`
	Name          string            `bson:"name" json:"name"`
	ManagerID     bson.ObjectID     `bson:"managerId,omitempty" json:"managerId,omitempty"`
	NameKey       string            `bson:"nameKey,omitempty" json:"-"`
	NodeID        bson.ObjectID     `bson:"nodeId" json:"nodeId"`
	ConfigID      bson.ObjectID     `bson:"configId" json:"configId"`
	ContainerID   string            `bson:"containerId,omitempty" json:"containerId,omitempty"`
	Address       string            `bson:"address,omitempty" json:"address,omitempty"`
	Allocations   []PortAllocation  `bson:"allocations,omitempty" json:"allocations,omitempty"`
	Status        string            `bson:"status" json:"status"`
	Error         string            `bson:"error,omitempty" json:"error,omitempty"`
	CPULimit      float64           `bson:"cpuLimit" json:"cpuLimit"`
	MemoryMB      int64             `bson:"memoryMB" json:"memoryMB"`
	DiskSizeBytes int64             `bson:"diskSizeBytes,omitempty" json:"diskSizeBytes,omitempty"`
	Volumes       []ServerVolume    `bson:"volumes,omitempty" json:"volumes,omitempty"`
	Variables     map[string]string `bson:"variables" json:"variables"`
	LaunchCommand string            `bson:"launchCommand,omitempty" json:"launchCommand,omitempty"`
	GameCommand   string            `bson:"gameCommand,omitempty" json:"gameCommand,omitempty"`
	CreatedAt     time.Time         `bson:"createdAt" json:"createdAt"`
	UpdatedAt     time.Time         `bson:"updatedAt" json:"updatedAt"`
}

type ServerVolume struct {
	ID        string    `bson:"id" json:"id"`
	Name      string    `bson:"name" json:"name"`
	MountPath string    `bson:"mountPath" json:"mountPath"`
	SizeBytes int64     `bson:"sizeBytes" json:"sizeBytes"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
}

type PortAllocation struct {
	Name      string `bson:"name" json:"name"`
	IP        string `bson:"ip" json:"ip"`
	Host      int    `bson:"host" json:"host"`
	Container int    `bson:"container" json:"container"`
	Protocol  string `bson:"protocol" json:"protocol"`
}

type Backup struct {
	ID        string        `bson:"_id" json:"id"`
	ServerID  bson.ObjectID `bson:"serverId" json:"serverId"`
	NodeID    bson.ObjectID `bson:"nodeId" json:"nodeId"`
	Image     string        `bson:"image" json:"image"`
	SizeBytes int64         `bson:"sizeBytes" json:"sizeBytes"`
	CreatedAt time.Time     `bson:"createdAt" json:"createdAt"`
}

type AuditLog struct {
	ID        bson.ObjectID  `bson:"_id,omitempty" json:"id"`
	UserID    *bson.ObjectID `bson:"userId,omitempty" json:"userId,omitempty"`
	Action    string         `bson:"action" json:"action"`
	Detail    string         `bson:"detail" json:"detail"`
	CreatedAt time.Time      `bson:"createdAt" json:"createdAt"`
}
