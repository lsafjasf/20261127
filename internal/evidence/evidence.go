// Package evidence 实现比对证据链:追加式 JSONL 日志,
// 每条记录包含前序哈希构成哈希链,并可用 ed25519 签名,
// 任何删改、重排、插入都会使校验失败。
package evidence

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Entry 是证据链中的一条记录。
type Entry struct {
	Seq        int             `json:"seq"`
	Time       time.Time       `json:"time"`
	Action     string          `json:"action"` // fingerprint / compare
	Subjects   []string        `json:"subjects"`
	FileHashes []string        `json:"file_hashes"` // 各文件整文件 SHA-256
	PayloadFPs []string        `json:"payload_fps"` // 各文件载荷指纹
	Verdict    string          `json:"verdict,omitempty"`
	Detail     json.RawMessage `json:"detail,omitempty"` // 完整指纹/比对报告
	PrevHash   string          `json:"prev_hash"`
	Hash       string          `json:"hash"` // 以上字段规范 JSON 的 SHA-256
	Signature  string          `json:"signature,omitempty"`
}

// hashPayload 返回用于计算条目哈希的规范 JSON(不含 Hash/Signature)。
func (e *Entry) hashPayload() []byte {
	cpy := *e
	cpy.Hash = ""
	cpy.Signature = ""
	data, _ := json.Marshal(cpy) // 结构体字段序固定,序列化确定
	return data
}

// ComputeHash 计算条目哈希。
func (e *Entry) ComputeHash() string {
	sum := sha256.Sum256(e.hashPayload())
	return hex.EncodeToString(sum[:])
}

// Load 读取证据链文件;文件不存在时返回空链。
func Load(path string) ([]*Entry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []*Entry
	for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("证据链第 %d 行解析失败: %w", i+1, err)
		}
		entries = append(entries, &e)
	}
	return entries, nil
}

// Append 构造并追加一条记录,返回新条目。
func Append(path string, e *Entry, priv ed25519.PrivateKey) (*Entry, error) {
	entries, err := Load(path)
	if err != nil {
		return nil, err
	}
	e.Seq = len(entries)
	e.Time = time.Now().UTC().Truncate(time.Second)
	if len(entries) > 0 {
		e.PrevHash = entries[len(entries)-1].Hash
	} else {
		e.PrevHash = strings.Repeat("0", 64) // 创世前哈希
	}
	e.Hash = e.ComputeHash()
	if priv != nil {
		sig := ed25519.Sign(priv, []byte(e.Hash))
		e.Signature = hex.EncodeToString(sig)
	}
	line, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return nil, err
	}
	return e, nil
}

// Verify 校验整条链:序号连续、哈希链接、哈希重算一致、签名有效。
// pub 为 nil 时跳过签名校验(但要求条目未签名)。
func Verify(entries []*Entry, pub ed25519.PublicKey) error {
	prev := strings.Repeat("0", 64)
	for i, e := range entries {
		if e.Seq != i {
			return fmt.Errorf("条目 %d 序号不连续(实际 %d)", i, e.Seq)
		}
		if e.PrevHash != prev {
			return fmt.Errorf("条目 %d 前序哈希断裂:链被删改或重排", i)
		}
		if e.Hash != e.ComputeHash() {
			return fmt.Errorf("条目 %d 哈希校验失败:内容被篡改", i)
		}
		if e.Signature != "" {
			if pub == nil {
				return fmt.Errorf("条目 %d 含签名但未提供公钥", i)
			}
			sig, err := hex.DecodeString(e.Signature)
			if err != nil || !ed25519.Verify(pub, []byte(e.Hash), sig) {
				return fmt.Errorf("条目 %d 签名无效", i)
			}
		}
		prev = e.Hash
	}
	return nil
}

// Keygen 生成 ed25519 密钥对并写入文件(hex 编码)。
func Keygen(privPath, pubPath string) error {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return err
	}
	if err := os.WriteFile(privPath, []byte(hex.EncodeToString(priv)), 0o600); err != nil {
		return err
	}
	return os.WriteFile(pubPath, []byte(hex.EncodeToString(pub)), 0o644)
}

// LoadKey 从 hex 文件读取私钥或公钥。
func LoadKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(strings.TrimSpace(string(data)))
}
