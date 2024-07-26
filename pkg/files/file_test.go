package files

import "testing"

func TestBackupIfExists(t *testing.T) {
	err := BackupIfExists("dev.yaml")
	if err != nil {
		t.Log(err.Error())
	} else {
		t.Log("OK")
	}
}
