package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// NewDeployID returns a deterministic-format unique identifier for a single
// deploy attempt. Format: YYYYMMDD-HHMMSS-XXXXXX (six-hex random suffix).
func NewDeployID() string {
	now := time.Now().UTC()
	var b [3]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s-%s-%s", now.Format("20060102"), now.Format("150405"), hex.EncodeToString(b[:]))
}

// ContainerName returns the M1-style container name for a service.
// M1 format: "decloud-<name>". M4 changes to include a deploy-id suffix —
// route all naming through this helper so the M4 migration touches one
// function body. See _ai/container-naming.md.
func ContainerName(serviceName string) string {
	return "decloud-" + serviceName
}

// ImageRef returns the image reference for a built service.
// Format: "decloud-<name>:<deploy-id>".
func ImageRef(serviceName, deployID string) string {
	return "decloud-" + serviceName + ":" + deployID
}
