package backup

import (
	"io"
	"os"
)

func walkSparseDataByScanning(file *os.File, size uint64, visit func(uint64, uint64) error) error {
	const blockSize = 64 * 1024
	buffer := make([]byte, blockSize)
	var runStart, runEnd uint64
	active := false
	flush := func() error {
		if active {
			return visit(runStart, runEnd-runStart)
		}
		return nil
	}
	for offset := uint64(0); offset < size; {
		length := uint64(blockSize)
		if size-offset < length {
			length = size - offset
		}
		count, err := file.ReadAt(buffer[:int(length)], int64(offset))
		if err != nil && !(err == io.EOF && count == int(length)) {
			return err
		}
		nonZero := false
		for _, value := range buffer[:int(length)] {
			if value != 0 {
				nonZero = true
				break
			}
		}
		if nonZero {
			if !active {
				runStart = offset
				active = true
			}
			runEnd = offset + length
		} else if active {
			if err := flush(); err != nil {
				return err
			}
			active = false
		}
		offset += length
	}
	return flush()
}
