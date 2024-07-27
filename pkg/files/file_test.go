package files

import "testing"

func TestBackupIfExists(t *testing.T) {
	err := BackupIfExists("test.txt")
	if err != nil {
		t.Log(err.Error())
	} else {
		t.Log("OK")
	}
}
