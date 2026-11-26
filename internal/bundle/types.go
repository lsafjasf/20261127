package bundle

import "time"

// Kind 标识交付包中的轨道类型。
type Kind string

const (
	KindAudio    Kind = "audio"
	KindSubtitle Kind = "subtitles"
	KindChapter  Kind = "chapters"
	KindCover    Kind = "cover"
)

// AllKinds 按固定顺序列出全部轨道类型，保证矩阵输出稳定。
var AllKinds = []Kind{KindAudio, KindSubtitle, KindChapter, KindCover}

// Track 描述扫描到的单个媒体文件。
type Track struct {
	Kind     Kind   `json:"kind"`
	Language string `json:"language"`
	// SourceAbs 是源文件的绝对路径，仅用于本地读取，不会写入清单。
	SourceAbs string `json:"-"`
	// RelPath 是相对交付包根目录的可移植路径（正斜杠）。
	RelPath   string `json:"path"`
	Codec     string `json:"codec"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256,omitempty"`
	Duplicate bool   `json:"duplicate,omitempty"`
	Chosen    bool   `json:"chosen,omitempty"`
}

// Cell 是矩阵中“轨道类型 x 语言”的一个格子。
type Cell struct {
	Present bool    `json:"present"`
	Tracks  []Track `json:"tracks,omitempty"`
	// Chosen 在存在多个候选时指向入选轨道的 RelPath。
	Chosen string `json:"chosen,omitempty"`
}

// Matrix 以轨道类型为行、语言为列组织全部候选轨道。
type Matrix struct {
	Languages []string                  `json:"languages"`
	Cells     map[Kind]map[string]*Cell `json:"cells"`
}

// FileEntry 记录进入交付包的文件及其校验信息。
type FileEntry struct {
	Path   string `json:"path"`
	Kind   Kind   `json:"kind"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Manifest 是交付包根目录下的可移植清单，只包含相对路径。
type Manifest struct {
	Schema    string            `json:"schema"`
	VideoID   string            `json:"video_id"`
	CreatedAt time.Time         `json:"created_at"`
	Matrix    Matrix            `json:"matrix"`
	Files     []FileEntry       `json:"files"`
	Warnings  []string          `json:"warnings,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// DuplicatePolicy 决定同一轨道类型出现重复语言时的行为。
type DuplicatePolicy string

const (
	// DuplicatePrefer 保留优先级最高的候选，其余标记为 duplicate。
	DuplicatePrefer DuplicatePolicy = "prefer"
	// DuplicateError 在出现重复语言时直接让打包失败。
	DuplicateError DuplicatePolicy = "error"
)

// MissingSubtitlePolicy 决定音频语言缺失字幕时的行为。
type MissingSubtitlePolicy string

const (
	// MissingSubtitleWarn 缺失字幕只产生警告，打包继续。
	MissingSubtitleWarn MissingSubtitlePolicy = "warn"
	// MissingSubtitleFail 缺失字幕视为打包失败。
	MissingSubtitleFail MissingSubtitlePolicy = "fail"
)

// Options 汇总一次打包任务的全部输入参数。
type Options struct {
	VideoID     string
	AudioDir    string
	SubtitleDir string
	ChapterDir  string
	CoverDir    string
	// Out 是最终交付包目录路径；与 ZipPath 二选一。
	Out string
	// ZipPath 非空时在目录包之外额外生成 zip 归档。
	ZipPath string

	DuplicatePolicy       DuplicatePolicy
	MissingSubtitlePolicy MissingSubtitlePolicy
	// IncludeDuplicates 为 true 时未入选的重复文件仍复制进交付包。
	IncludeDuplicates bool
	// Force 允许覆盖已存在的输出路径。
	Force bool

	Metadata map[string]string
}

func (o Options) dirFor(k Kind) string {
	switch k {
	case KindAudio:
		return o.AudioDir
	case KindSubtitle:
		return o.SubtitleDir
	case KindChapter:
		return o.ChapterDir
	case KindCover:
		return o.CoverDir
	default:
		return ""
	}
}
