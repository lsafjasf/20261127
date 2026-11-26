package casegen

import (
	"os"
	"path/filepath"

	"evidencechain/internal/containerx"
)

// writeFieldReturns builds the messy real-world intake: renamed loose files,
// shuffled/renamed archive members, cross-container relocation, localized
// tampering, a high-similarity parody and an unknown binary.
func (f *fixture) writeFieldReturns() error {
	base := filepath.Join(f.dir, "30-field-returns")

	loose := map[string][]byte{
		"未命名_扫描件_01.dat":    f.ev1, // exact EV-001, only the name changed
		"客户导出_20261126.csv": f.ev2, // exact EV-002, renamed
		"值班日志_外传修订版.txt":    f.tampered,
		"值班日志_同题仿作.txt":     f.parody,
		"设备缓存_dc88.bin":     f.unknown,
	}
	for name, data := range loose {
		if err := os.WriteFile(filepath.Join(base, name), data, 0o644); err != nil {
			return err
		}
	}

	// Same four contents as sealed-pack.zip, but entries renamed and order
	// shuffled. The order here is deliberately not EV-001..EV-004.
	shuffled := []containerx.Entry{
		{Name: "dump_fw.bin", Data: f.ev3},
		{Name: "memo_final.txt", Data: f.ev1},
		{Name: "cap_0926.png", Data: f.ev4},
		{Name: "customers_q4.csv", Data: f.ev2},
	}
	if err := containerx.WriteZip(filepath.Join(base, "backup_20261126.zip"), shuffled); err != nil {
		return err
	}

	// Mail attachment: one tampered "final" log plus the intact customer CSV.
	mail := []containerx.Entry{
		{Name: "日志终稿(请查收).txt", Data: f.tampered},
		{Name: "客户名单(最新).csv", Data: f.ev2},
	}
	if err := containerx.WriteZip(filepath.Join(base, "邮件附件_日志终稿.zip"), mail); err != nil {
		return err
	}

	// Cross-container relocation: EV-003 and EV-004 reappear in a tar.
	relocated := []containerx.Entry{
		{Name: "blob_a.bin", Data: f.ev3},
		{Name: "pic_b.png", Data: f.ev4},
	}
	return containerx.WriteTar(filepath.Join(base, "现场资料_打包.tar"), relocated)
}
