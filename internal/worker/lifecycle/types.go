package lifecycle

import domaintypes "github.com/iw2rmb/ploy/internal/domain/types"

// NodeCapacity represents available node resources for scheduling.
type NodeCapacity struct {
	CPUFreeMillis  domaintypes.CPUmilli
	CPUTotalMillis domaintypes.CPUmilli
	MemFreeBytes   domaintypes.Bytes
	MemTotalBytes  domaintypes.Bytes
	DiskFreeBytes  domaintypes.Bytes
	DiskTotalBytes domaintypes.Bytes
}
