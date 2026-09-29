//go:build linux

package backup

import (
	"errors"
	"os"
	"syscall"
)

func walkSparseData(file *os.File, size uint64, visit func(uint64, uint64) error) error {
	var offset uint64
	for offset < size {
		data, err := syscall.Seek(int(file.Fd()), int64(offset), 3) // SEEK_DATA
		if errors.Is(err, syscall.ENXIO) {
			return nil
		}
		if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.ENOSYS) {
			return walkSparseDataByScanning(file, size, visit)
		}
		if err != nil {
			return err
		}
		if uint64(data) >= size {
			return nil
		}
		hole, err := syscall.Seek(int(file.Fd()), data, 4) // SEEK_HOLE
		if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.ENOSYS) {
			return walkSparseDataByScanning(file, size, visit)
		}
		if err != nil {
			return err
		}
		end := uint64(hole)
		if end > size {
			end = size
		}
		if end <= uint64(data) {
			return errors.New("filesystem returned an invalid sparse extent")
		}
		if err := visit(uint64(data), end-uint64(data)); err != nil {
			return err
		}
		offset = end
	}
}
