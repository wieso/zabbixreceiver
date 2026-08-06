package metrics

import "testing"

func TestNameSanitizesZabbixItemKeys(t *testing.T) {
	tests := []struct {
		prefix  string
		itemKey string
		want    string
	}{
		{"zabbix_", "system.cpu.util", "zabbix_system_cpu_util"},
		{"zabbix_", `vfs.fs.size[/,free]`, "zabbix_vfs_fs_size___free"},
		{"", "9bad", "_9bad"},
		{"zabbix_", "ends...", "zabbix_ends"},
		{"zabbix_", "cpu.温度.util", "zabbix_cpu____util"},
		{"9", "key", "_9key"},
		{"zabbix_", "", "zabbix"},
	}

	for _, tt := range tests {
		t.Run(tt.itemKey, func(t *testing.T) {
			got, err := Name(tt.prefix, tt.itemKey)
			if err != nil {
				t.Fatalf("Name(%q, %q) returned error: %v", tt.prefix, tt.itemKey, err)
			}
			if got != tt.want {
				t.Fatalf("Name(%q, %q) = %q, want %q", tt.prefix, tt.itemKey, got, tt.want)
			}
		})
	}
}

func TestNameRejectsAnEmptyResult(t *testing.T) {
	for _, itemKey := range []string{"", "..."} {
		t.Run(itemKey, func(t *testing.T) {
			_, err := Name("", itemKey)
			if err == nil {
				t.Fatal("Name returned nil error for an empty sanitized name")
			}
		})
	}
}
