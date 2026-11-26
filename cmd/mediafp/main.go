// mediafp —— 媒体载荷/结构指纹工具:区分重封装、元数据修改与真实帧替换,
// 并生成可验证的证据链。
//
// 用法:
//
//	mediafp fp <文件>                          计算四级指纹
//	mediafp compare <旧文件> <新文件> [选项]    比对并判定
//	mediafp verify-chain <证据链> [--pub 公钥]  校验证据链
//	mediafp keygen <私钥文件> <公钥文件>        生成签名密钥对
//
// compare 选项:
//
//	--chain <证据链文件>   将本次比对追加进证据链
//	--sign  <私钥文件>     用 ed25519 私钥对证据条目签名
package main

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"

	"mediafp/internal/evidence"
	"mediafp/internal/fingerprint"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "fp":
		err = cmdFP(os.Args[2:])
	case "compare":
		err = cmdCompare(os.Args[2:])
	case "verify-chain":
		err = cmdVerifyChain(os.Args[2:])
	case "keygen":
		err = cmdKeygen(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `用法:
  mediafp fp <文件>                           计算四级指纹
  mediafp compare <旧> <新> [--chain C] [--sign K]  比对判定
  mediafp verify-chain <证据链> [--pub 公钥]    校验证据链
  mediafp keygen <私钥> <公钥>                  生成密钥对`)
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func cmdFP(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("fp 需要 1 个文件参数")
	}
	m, err := fingerprint.Compute(args[0])
	if err != nil {
		return err
	}
	return printJSON(m.FP)
}

func cmdCompare(args []string) error {
	var files, chain, signKey []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--chain":
			i++
			chain = append(chain, args[i])
		case "--sign":
			i++
			signKey = append(signKey, args[i])
		default:
			files = append(files, args[i])
		}
	}
	if len(files) != 2 {
		return fmt.Errorf("compare 需要 2 个文件参数")
	}
	a, err := fingerprint.Compute(files[0])
	if err != nil {
		return fmt.Errorf("旧文件: %w", err)
	}
	b, err := fingerprint.Compute(files[1])
	if err != nil {
		return fmt.Errorf("新文件: %w", err)
	}
	report := fingerprint.Compare(a, b)
	fmt.Fprint(os.Stderr, report.String())

	if len(chain) > 0 {
		var priv ed25519.PrivateKey
		if len(signKey) > 0 {
			key, err := evidence.LoadKey(signKey[0])
			if err != nil {
				return err
			}
			priv = ed25519.PrivateKey(key)
		}
		detail, _ := json.Marshal(report)
		entry, err := evidence.Append(chain[0], &evidence.Entry{
			Action:     "compare",
			Subjects:   files,
			FileHashes: []string{a.FP.WholeFile, b.FP.WholeFile},
			PayloadFPs: []string{a.FP.Payload, b.FP.Payload},
			Verdict:    string(report.Verdict),
			Detail:     detail,
		}, priv)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "证据已入链: %s 条目 #%d 哈希 %s\n",
			chain[0], entry.Seq, entry.Hash[:16])
	}
	return printJSON(report)
}

func cmdVerifyChain(args []string) error {
	var chainPath, pubPath string
	for i := 0; i < len(args); i++ {
		if args[i] == "--pub" {
			i++
			pubPath = args[i]
		} else {
			chainPath = args[i]
		}
	}
	if chainPath == "" {
		return fmt.Errorf("verify-chain 需要证据链文件")
	}
	entries, err := evidence.Load(chainPath)
	if err != nil {
		return err
	}
	var pub ed25519.PublicKey
	if pubPath != "" {
		key, err := evidence.LoadKey(pubPath)
		if err != nil {
			return err
		}
		pub = ed25519.PublicKey(key)
	}
	if err := evidence.Verify(entries, pub); err != nil {
		return err
	}
	fmt.Printf("证据链校验通过: %d 条记录,链完整\n", len(entries))
	return nil
}

func cmdKeygen(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("keygen 需要 <私钥文件> <公钥文件>")
	}
	if err := evidence.Keygen(args[0], args[1]); err != nil {
		return err
	}
	fmt.Println("密钥对已生成:", args[0], args[1])
	return nil
}
