//go:build !linux

package backup

import "os"

func walkSparseData(file *os.File, size uint64, visit func(uint64, uint64) error) error {
	return walkSparseDataByScanning(file, size, visit)
}
