// Package cli implements the evidencechain command surface. It depends on
// the Go standard library only and builds fully offline.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"evidencechain/internal/casegen"
	"evidencechain/internal/chain"
	"evidencechain/internal/containerx"
	"evidencechain/internal/digestx"
	"evidencechain/internal/fuzzyx"
)

// Run executes the CLI and terminates the process with an appropriate code.
func Run() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	var err error
	switch os.Args[1] {
	case "init-case":
		err = cmdInitCase(os.Args[2:])
	case "index":
		err = cmdIndex(os.Args[2:])
	case "verify":
		err = cmdVerify(os.Args[2:])
	case "report":
		err = cmdReport(os.Args[2:])
	case "hash":
		err = cmdHash(os.Args[2:])
	case "container":
		err = cmdContainer(os.Args[2:])
	case "compare":
		err = cmdCompare(os.Args[2:])
	case "version":
		fmt.Printf("%s %s (schema %s)\n", "evidencechain", "1.0.0", "evidencechain/v1")
	default:
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `evidencechain — 可验证取证证据链工具（纯标准库，可离线构建）

用法:
  evidencechain init-case --dir case [--force]   生成确定性参考案例
  evidencechain index --case case [--chunk-size N]
                                                 从 00-originals 构建哈希索引
  evidencechain verify --case case [--write-json case/out/verification.json]
                                                 校验完整证据链与人工判读
  evidencechain report --case case --out FORENSIC-REPORT.md
                                                 生成中文取证报告
  evidencechain hash <file> [--chunks N]          输出 SHA-256/块/模糊指纹
  evidencechain container <archive>               输出 zip/tar 规范清单
  evidencechain compare <a> <b>                   比较两文件的相似性信号
`)
}

func cmdInitCase(args []string) error {
	fs := flag.NewFlagSet("init-case", flag.ContinueOnError)
	dir := fs.String("dir", "case", "案例目录")
	_ = fs.Bool("force", false, "允许写入已存在目录（不删除 review/ 人工判读）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := casegen.Generate(*dir); err != nil {
		return err
	}
	fmt.Println("reference case materialized at", *dir)
	return nil
}

func cmdIndex(args []string) error {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	c := fs.String("case", "case", "案例目录")
	chunk := fs.Int("chunk-size", 256, "对齐块大小（字节）")
	caseID := fs.String("case-id", casegen.CaseID, "案例编号")
	if err := fs.Parse(args); err != nil {
		return err
	}
	idx, err := chain.BuildIndex(*c, *caseID, *chunk)
	if err != nil {
		return err
	}
	fmt.Printf("indexed %d originals, chunk-size=%d -> %s\n", len(idx.Entries), idx.ChunkSize, chain.IndexRel)
	return nil
}

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	c := fs.String("case", "case", "案例目录")
	out := fs.String("write-json", "", "将 verification JSON 写入该路径")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := chain.Verify(*c)
	if err != nil {
		return err
	}
	if *out != "" {
		if err := chain.WriteJSON(*out, res); err != nil {
			return err
		}
		fmt.Println("verification JSON written:", *out)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res.Summary); err != nil {
		return err
	}
	if !res.OK {
		fmt.Fprintln(os.Stderr, "CHAIN VERIFICATION FAILED:", len(res.Errors), "error(s)")
		for _, e := range res.Errors {
			fmt.Fprintln(os.Stderr, "  -", e)
		}
		os.Exit(3)
	}
	fmt.Println("CHAIN OK — errors=0, warnings=", len(res.Warnings))
	for _, w := range res.Warnings {
		fmt.Println("  warn:", w)
	}
	return nil
}

func cmdHash(args []string) error {
	fs := flag.NewFlagSet("hash", flag.ContinueOnError)
	chunks := fs.Int("chunks", 0, "同时输出该块大小的对齐块哈希")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("hash requires exactly one file")
	}
	path := fs.Arg(0)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out := map[string]any{
		"path":   path,
		"size":   len(data),
		"sha256": digestx.SHA256Bytes(data),
		"fuzzy":  fuzzyx.Sum(data),
	}
	if *chunks > 0 {
		cs := digestx.ChunkBytes(data, *chunks)
		out["chunk_size"] = *chunks
		out["chunk_root"] = digestx.ChunkRoot(cs)
		out["chunks"] = cs
	}
	return printJSON(out)
}

func cmdContainer(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("container requires exactly one archive path")
	}
	fileSHA, m, err := containerx.InspectFile(args[0])
	if err != nil {
		return err
	}
	out := map[string]any{
		"path": args[0], "file_sha256": fileSHA,
		"format": m.Format, "memberset_sha256": m.MemberSetSHA256,
		"pathset_sha256": m.PathSetSHA256, "members": m.Members,
	}
	return printJSON(out)
}

func cmdCompare(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("compare requires <a> <b>")
	}
	a, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	b, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	r := fuzzyx.Compare(a, b)
	sa, sb := fuzzyx.SimHash(a), fuzzyx.SimHash(b)
	return printJSON(map[string]any{
		"a": args[0], "b": args[1],
		"a_sha256": digestx.SHA256Bytes(a), "b_sha256": digestx.SHA256Bytes(b),
		"a_simhash": fmt.Sprintf("%016x", sa), "b_simhash": fmt.Sprintf("%016x", sb),
		"similarity": r,
		"policy":     "fuzzy similarity is NOT identity and NOT proof of common source",
	})
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
