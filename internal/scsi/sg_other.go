//go:build !linux

package scsi

import "errors"

func Read(device string, cdb []byte, allocation int) ([]byte, error) {
	return nil, errors.New("SCSI generic access needs Linux")
}

func Inquiry(device string) (string, string, string, error) {
	return "", "", "", errors.New("SCSI generic access needs Linux")
}
