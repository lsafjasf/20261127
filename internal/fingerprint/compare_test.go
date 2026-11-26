package fingerprint_test

import (
	"os"
	"path/filepath"
	"testing"

	"mediafp/internal/fingerprint"
	"mediafp/internal/testutil"
)

// makeSamples 生成确定性伪随机样本数据。
func makeSamples(n, size int, seed byte) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		buf := make([]byte, size)
		for j := range buf {
			buf[j] = byte(i*31 + j*7 + int(seed))
		}
		out[i] = buf
	}
	return out
}

func baseTracks() []testutil.TrackSpec {
	return []testutil.TrackSpec{
		{Handler: "vide", Codec: "avc1", TrackID: 1, Timescale: 90000,
			Delta: 3000, Samples: makeSamples(8, 500, 1), ChunkSize: 3},
		{Handler: "soun", Codec: "mp4a", TrackID: 2, Timescale: 48000,
			Delta: 1024, Samples: makeSamples(10, 128, 2), ChunkSize: 4},
	}
}

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustCompute(t *testing.T, path string) *fingerprint.Media {
	t.Helper()
	m, err := fingerprint.Compute(path)
	if err != nil {
		t.Fatalf("Compute(%s): %v", path, err)
	}
	return m
}

// 场景 1:正常重封装 —— 盒顺序颠倒、增删填充、chunk 重排、stco 变化。
func TestRemux(t *testing.T) {
	a := writeFile(t, "a.mp4", testutil.Build(baseTracks(),
		testutil.Options{ModTime: 1000}))
	remuxTracks := baseTracks()
	remuxTracks[0].ChunkSize = 2 // 改变 chunk 布局
	remuxTracks[1].ChunkSize = 1
	b := writeFile(t, "b.mp4", testutil.Build(remuxTracks,
		testutil.Options{MoovFirst: true, AddFree: true, ModTime: 1000}))

	ma, mb := mustCompute(t, a), mustCompute(t, b)
	if ma.FP.WholeFile == mb.FP.WholeFile {
		t.Fatal("整文件哈希应不同")
	}
	if ma.FP.Payload != mb.FP.Payload {
		t.Fatal("重封装后载荷指纹应一致")
	}
	if ma.FP.Layout == mb.FP.Layout {
		t.Fatal("重封装后布局指纹应不同")
	}
	r := fingerprint.Compare(ma, mb)
	if r.Verdict != fingerprint.Remux {
		t.Fatalf("判定 = %s,期望 REMUX", r.Verdict)
	}
}

// 场景 2:元数据修改 —— 仅改时间戳并附加 udta 标签。
func TestMetadataModified(t *testing.T) {
	a := writeFile(t, "a.mp4", testutil.Build(baseTracks(),
		testutil.Options{ModTime: 1000}))
	b := writeFile(t, "b.mp4", testutil.Build(baseTracks(),
		testutil.Options{ModTime: 2000, Udta: []byte("标题:测试视频")}))

	ma, mb := mustCompute(t, a), mustCompute(t, b)
	if ma.FP.Payload != mb.FP.Payload {
		t.Fatal("改元数据不应影响载荷指纹")
	}
	if ma.FP.Layout != mb.FP.Layout {
		t.Fatal("改元数据不应影响布局指纹")
	}
	if ma.FP.Metadata == mb.FP.Metadata {
		t.Fatal("元数据指纹应不同")
	}
	r := fingerprint.Compare(ma, mb)
	if r.Verdict != fingerprint.MetadataModified {
		t.Fatalf("判定 = %s,期望 METADATA_MODIFIED", r.Verdict)
	}
}

// 场景 3:真实帧替换 —— 替换视频轨第 5 帧内容(长度不变)。
func TestContentReplaced(t *testing.T) {
	a := writeFile(t, "a.mp4", testutil.Build(baseTracks(),
		testutil.Options{ModTime: 1000}))
	evil := baseTracks()
	for j := range evil[0].Samples[4] {
		evil[0].Samples[4][j] ^= 0xFF // 篡改第 5 个视频样本
	}
	b := writeFile(t, "b.mp4", testutil.Build(evil,
		testutil.Options{ModTime: 1000}))

	ma, mb := mustCompute(t, a), mustCompute(t, b)
	r := fingerprint.Compare(ma, mb)
	if r.Verdict != fingerprint.ContentReplaced {
		t.Fatalf("判定 = %s,期望 CONTENT_REPLACED", r.Verdict)
	}
	if len(r.TrackDiffs) != 1 {
		t.Fatalf("应只有 1 轨差异,实际 %d", len(r.TrackDiffs))
	}
	d := r.TrackDiffs[0]
	if d.Handler != "vide" || d.FirstDiffIndex != 4 {
		t.Fatalf("差异定位错误: %+v", d)
	}
	if len(d.DiffIndices) != 1 || d.DiffIndices[0] != 4 {
		t.Fatalf("差异样本列表错误: %+v", d)
	}
}

// 场景 4:完全相同的文件。
func TestIdentical(t *testing.T) {
	data := testutil.Build(baseTracks(), testutil.Options{ModTime: 1000})
	a := writeFile(t, "a.mp4", data)
	b := writeFile(t, "b.mp4", data)
	r := fingerprint.Compare(mustCompute(t, a), mustCompute(t, b))
	if r.Verdict != fingerprint.Identical {
		t.Fatalf("判定 = %s,期望 IDENTICAL", r.Verdict)
	}
}

// 场景 5:增删帧也属内容变化。
func TestFrameCountChanged(t *testing.T) {
	a := writeFile(t, "a.mp4", testutil.Build(baseTracks(),
		testutil.Options{ModTime: 1000}))
	short := baseTracks()
	short[0].Samples = short[0].Samples[:6] // 删掉 2 帧
	b := writeFile(t, "b.mp4", testutil.Build(short,
		testutil.Options{ModTime: 1000}))
	r := fingerprint.Compare(mustCompute(t, a), mustCompute(t, b))
	if r.Verdict != fingerprint.ContentReplaced {
		t.Fatalf("判定 = %s,期望 CONTENT_REPLACED", r.Verdict)
	}
	if r.TrackDiffs[0].SamplesA != 8 || r.TrackDiffs[0].SamplesB != 6 {
		t.Fatalf("样本计数证据错误: %+v", r.TrackDiffs[0])
	}
}
