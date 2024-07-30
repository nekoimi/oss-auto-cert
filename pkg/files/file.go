package files

import (
	"errors"
	"log"
	"os"
	"time"
)

// Exists 判断文件是否存在
func Exists(f string) (bool, error) {
	if _, err := os.Stat(f); err == nil {
		// f exists
		return true, nil
	} else if errors.Is(err, os.ErrNotExist) {
		// f does not exists
		return false, nil
	} else {
		// f stat err, return false and err
		return false, err
	}
}

func BackupIfExists(name string) error {
	if exists, err := Exists(name); err != nil {
		return err
	} else if exists {
		newname := name + "." + "backup" + "-" + time.Now().Format("20060102150405")
		err = os.Rename(name, newname)
		if err != nil {
			return err
		}
	}
	return nil
}

func ReadIfExists(name string) (bool, []byte) {
	if exists, err := Exists(name); err != nil {
		return false, make([]byte, 0)
	} else if exists {
		b, err := os.ReadFile(name)
		if err != nil {
			log.Printf("read file %s error: %s\n", name, err)
			return false, make([]byte, 0)
		}

		return true, b
	}

	return false, make([]byte, 0)
}
